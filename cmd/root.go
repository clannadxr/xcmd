/*
Copyright © 2019 clannadxr <clannadxr@hotmail.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/clannadxr/xcmd/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type config struct {
	Concurrent          bool
	Command             string
	Parameters          string
	OutputCheckDeadline time.Duration
	ContextDeadline     time.Duration
	MaxProcess          int
	KnockInterval       time.Duration
}

type xcmd struct {
	command        *exec.Cmd
	name           string
	out            *util.Buffer
	err            error
	outputCheck    func(*xcmd)
	handleShutdown func(*xcmd)
	stop           chan struct{}
}

type xcmdWrapper struct {
	mu       sync.RWMutex
	names    []string
	commands map[string]xcmd
}

var cfg config

var rootCmd = &cobra.Command{
	Use:   "xcmd",
	Short: "并发/顺序运行多脚本",
	Run:   runXCMD,
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.Flags().BoolP("concurrent", "c", true, "the commands will be executed currently if true")
	rootCmd.Flags().String("command", "", "the commands with wildcard like: /usr/bin/php -f ./phpscripts/echo.php command test --site_id={{.}}")
	rootCmd.Flags().String("parameters", "", "the parameters for the command's wildcard, like: 1,2,3,4")
	rootCmd.Flags().Int("outputCheckDeadline", 30, "if the script outputs nothing within the deadline, it will be killed")
	rootCmd.Flags().Int("contextDeadline", 60, "all processes will be killed if deadline reaches")
	rootCmd.Flags().Int("maxProcess", 10, "the maximum number of child process concurrently")
	rootCmd.Flags().Int("knockInterval", 2, "while waiting for the scripts, it will output anything in case of the ZOMBIE CHECK in this interval")
}

func runXCMD(cmd *cobra.Command, args []string) {
	xCMDs, err := parseCommands()
	if err != nil {
		log.Println(err)
		return
	}

	var (
		wg       sync.WaitGroup
		token    chan struct{}
		stopChan chan struct{}
	)
	stopChan = make(chan struct{})
	token = make(chan struct{}, cfg.MaxProcess) //最多并发 x 个脚本进程
	knock(stopChan)
	for _, name := range xCMDs.names {
		token <- struct{}{}
		wg.Add(1)
		go func(commandName string) {
			fmt.Printf("正在执行脚本: %s\n", commandName)
			defer func() {
				wg.Done()
				<-token
			}()
			xCMDs.mu.RLock()
			xCMD := xCMDs.commands[commandName]
			xCMDs.mu.RUnlock()
			xCMD.stop = make(chan struct{})
			xCMD.outputCheck(&xCMD)
			xCMD.handleShutdown(&xCMD)
			xCMD.err = xCMD.command.Run()
			close(xCMD.stop)
			xCMDs.mu.Lock()
			xCMDs.commands[commandName] = xCMD
			xCMDs.mu.Unlock()
		}(name)
	}
	wg.Wait()
	close(stopChan)
	//全部结束
	var allSuccess = true
	for _, name := range xCMDs.names {
		xCMD := xCMDs.commands[name]
		if xCMD.err == nil {
			log.Printf("脚本 %s 正常执行\n", xCMD.name)
		} else {
			log.Printf("[error] 脚本 %s 异常执行: %s\n", xCMD.name, xCMD.err)
			allSuccess = false
		}
		log.Printf("脚本 %s 的输出如下:\n", xCMD.name)
		fmt.Println(xCMD.out.String())
	}
	if !allSuccess {
		os.Exit(2)
	}
}

func parseCommands() (*xcmdWrapper, error) {
	if !cfg.Concurrent {
		cfg.MaxProcess = 1
	}
	parameters := strings.Split(cfg.Parameters, ",")

	if cfg.Concurrent {
		log.Println("以下脚本将以并发形式执行: ")
	} else {
		log.Println("以下脚本将以顺序形式执行: ")
	}
	var cmdStrs []string
	if len(parameters) > 0 {
		tmpl, err := template.New("cmd").Parse(cfg.Command)
		if err != nil {
			return nil, fmt.Errorf("解析command模板错误 :%s", err)
		}
		for _, v := range parameters {
			bf := bytes.Buffer{}
			err = tmpl.Execute(&bf, v)
			if err != nil {
				return nil, fmt.Errorf("模板执行出错: %s", err)
			}
			if bf.Len() == 0 {
				continue
			}
			cmdStr := bf.String()
			cmdStrs = append(cmdStrs, cmdStr)
			log.Println(cmdStr)
		}
	} else {
		cmdStrs = append(cmdStrs, cfg.Command)
		log.Println(cfg.Command)
	}
	if len(cmdStrs) == 0 {
		return nil, fmt.Errorf("[error] 没有要执行的脚本")
	}

	var xCMDs = new(xcmdWrapper)
	xCMDs.commands = make(map[string]xcmd, len(cmdStrs))
	xCMDs.names = cmdStrs
	for _, cmdStr := range xCMDs.names {
		// 1. 脚本超时 直接 kill 掉
		// 2. 脚本一定时间无输出， kill 掉
		args := strings.Split(cmdStr, " ")
		cmd := exec.Command(args[0], args[1:]...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		out := &util.Buffer{}
		cmd.Stdout = out
		cmd.Stderr = out
		outputCheck := func(cmd *xcmd) {
			go func() {
				oldLen := 0
				for {
					time.Sleep(time.Second * cfg.OutputCheckDeadline)
					select {
					case <-cmd.stop:
						return
					default:
					}
					newLen := len(cmd.out.Bytes())
					if newLen == oldLen {
						// 无输出
						log.Printf("脚本 %s 因为长时间没有输出，被直接关闭!", cmd.name)
						syscall.Kill(-cmd.command.Process.Pid, syscall.SIGKILL)
						return
					} else {
						oldLen = newLen
					}
				}
			}()
		}
		ctx := interruptWatcher(context.Background())
		ctx, _ = context.WithDeadline(ctx, time.Now().Add(time.Second*cfg.ContextDeadline))
		handleShutdown := func(cmd *xcmd) {
			go func() {
				<-ctx.Done()
				if cmd.command.Process == nil || cmd.command.ProcessState != nil {
					return
				}
				syscall.Kill(-cmd.command.Process.Pid, syscall.SIGKILL)
			}()
		}
		xCMDs.commands[cmdStr] = xcmd{
			command:        cmd,
			out:            out,
			name:           cmdStr,
			outputCheck:    outputCheck,
			handleShutdown: handleShutdown,
		}
	}
	return xCMDs, nil
}

func knock(ch <-chan struct{}) {
	go func(done <-chan struct{}) {
		for {
			time.Sleep(time.Second * cfg.KnockInterval)
			select {
			case <-done:
				return
			default:
				log.Println("等待所有脚本执行中....")
			}
		}
	}(ch)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func initConfig() {

	err := viper.BindPFlags(rootCmd.Flags())
	if err != nil {
		panic(err)
	}
	err = viper.Unmarshal(&cfg)
	if err != nil {
		panic(err)
	}
}

// 用于生成一个带cancel的ctx，并拦截系统的interrupt，调用cancel 以方便应用做退出前的工作
func interruptWatcher(pctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(pctx)
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGINT, os.Kill, syscall.SIGKILL, syscall.SIGTERM)
		sig := <-ch
		fmt.Printf("got %s signal, shutting down\n", sig)
		cancel()
	}()
	return ctx
}
