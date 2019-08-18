<?php

function echoLine(){
    echo '这里是PHP脚本内的输出，正在执行的脚本是: ';
    echo join(' ', $_SERVER['argv']);
    echo "\n";
}


for($i=0;$i<5;$i++) {
    echoLine();
    sleep(1);
}

