# vi: ft=make
SHELL:=/bin/bash


.PHONY: release

release: release.build release.tag


release.build:
		@echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>make $@<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<"
		@echo -e '\n building darwin version...'
		GOOS=darwin go build -o bin/xcmd_darwin main.go
		@echo -e '\n building linux version...'
		GOOS=linux go build -o bin/xcmd_linux main.go
		@echo -e "\n"
release.tag:
		@echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>make $@<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<"
		./bin/release.sh
