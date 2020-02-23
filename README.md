## protoc-gen-apex

A protoc plugin for Salesforce Apex

## Install

```bash
$ go get github.com/tzmfreedom/protoc-gen-apex
```

## Usage

generate class files with following command
```bash
$ protoc -I. --apex_out=. target.proto
```
