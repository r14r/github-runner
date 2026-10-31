package main

import (
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestVersionAndHelp(t *testing.T) {
 cwd:=t.TempDir()
 for _,args:=range [][]string{{"version"},{"--help"},nil} {
  if err:=run(args,filepath.Join(cwd,"bin"),cwd);err!=nil {t.Fatalf("%v: %v",args,err)}
 }
}
func TestMissingAssets(t *testing.T) {
 t.Setenv("GITHUB_RUNNER_CLI_ENV","")
 tmp:=t.TempDir()
 if _,err:=assetsDir(filepath.Join(tmp,"bin"),tmp);err==nil {t.Fatal("missing runtime assets accepted")}
}
func TestInvalidCommands(t *testing.T) {
 t.Setenv("GITHUB_RUNNER_CLI_ENV","")
 cwd,err:=os.Getwd();if err!=nil {t.Fatal(err)}
 root:=filepath.Join(cwd,"..","..")
 for _,args:=range [][]string{{"runner"},{"runner","unsupported"},{"runner","download"},{"host"},{"host","unknown"}} {
  if err:=run(args,filepath.Join(root,"dist","github-runner-cli"),root);err==nil {t.Fatalf("accepted invalid args %v",args)}
 }
}
func TestAllEntrypointsExist(t *testing.T) {
 root:=filepath.Join("..","..")
 for _,file:=range runnerActions {
  if _,err:=os.Stat(filepath.Join(root,"runner",file));err!=nil {t.Fatalf("missing %s: %v",file,err)}
 }
 for _,file:=range hostActions {
  if _,err:=os.Stat(filepath.Join(root,file));err!=nil {t.Fatalf("missing %s: %v",file,err)}
 }
}

func TestShortCommandsDispatchAndInstallOrder(t *testing.T) {
 root:=t.TempDir()
 runner:=filepath.Join(root,"runner")
 if err:=os.MkdirAll(runner,0755);err!=nil {t.Fatal(err)}
 for _,name:=range []string{"01-download.sh","02-configure.sh","04-run.sh"} {
  if err:=os.WriteFile(filepath.Join(runner,name),[]byte("# test stub\n"),0644);err!=nil{t.Fatal(err)}
 }
 mockBin:=filepath.Join(root,"mock-bin")
 if err:=os.Mkdir(mockBin,0755);err!=nil{t.Fatal(err)}
 logfile:=filepath.Join(root,"calls.log")
 fakeBash:=[]byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$RUNNER_TEST_CALLS\"\n")
 if err:=os.WriteFile(filepath.Join(mockBin,"bash"),fakeBash,0755);err!=nil{t.Fatal(err)}
 t.Setenv("PATH",mockBin+string(os.PathListSeparator)+os.Getenv("PATH"))
 t.Setenv("GITHUB_RUNNER_CLI_ENV",root)
 t.Setenv("RUNNER_TEST_CALLS",logfile)
 exe:=filepath.Join(root,"bin")
 if err:=run([]string{"install","r14r/BookBuilder","--allow-public"},exe,root);err!=nil{t.Fatal(err)}
 for _,command:=range []string{"start","stop","Status","Monitor"} {
  if err:=run([]string{command,"r14r/BookBuilder"},exe,root);err!=nil{t.Fatal(err)}
 }
 b,err:=os.ReadFile(logfile);if err!=nil{t.Fatal(err)}
 lines:=strings.Split(strings.TrimSpace(string(b)),"\n")
 expected:=[]string{
  filepath.Join(runner,"01-download.sh")+" r14r/BookBuilder",
  filepath.Join(runner,"02-configure.sh")+" r14r/BookBuilder --if-needed --allow-public",
  filepath.Join(runner,"04-run.sh")+" r14r/BookBuilder install",
  filepath.Join(runner,"04-run.sh")+" r14r/BookBuilder start",
  filepath.Join(runner,"04-run.sh")+" r14r/BookBuilder stop",
  filepath.Join(runner,"04-run.sh")+" r14r/BookBuilder status",
  filepath.Join(runner,"04-run.sh")+" r14r/BookBuilder monitor",
 }
 if len(lines)!=len(expected) {t.Fatalf("commands %q",lines)}
 for i,want:=range expected {
  if lines[i]!=want{t.Errorf("command %d: got %q, expected %q",i,lines[i],want)}
 }
}
func TestShortInstallRejectsUnsupportedOptions(t *testing.T) {
 root:=t.TempDir()
 t.Setenv("GITHUB_RUNNER_CLI_ENV",root)
 for _,args:=range [][]string{
  {"install"}, {"install","r14r/BookBuilder","--unknown"},
  {"install","r14r/BookBuilder","--name"}, {"status"},
  {"start","r14r/BookBuilder","unexpected"},
 }{
  if err:=run(args,filepath.Join(root,"bin"),root);err==nil{t.Errorf("accepted invalid args %q",args)}
 }
}
