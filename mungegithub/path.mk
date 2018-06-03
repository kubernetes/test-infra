TARGET:=$(shell basename $(shell pwd))
APP:=$(shell basename $(shell dirname $(shell dirname $(shell pwd))))
READONLY ?= false

include ../../../Makefile
