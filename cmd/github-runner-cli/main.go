package main

import (
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
)

var runnerActions = map[string]string{
 "download":"01-download.sh", "register":"02-configure.sh",
 "configure":"02-configure.sh", "setup":"03-setup.sh",
 "pr":"03-setup.sh", "install-service":"04-run.sh",
 "start":"04-run.sh", "stop":"04-run.sh", "status":"04-run.sh",
 "monitor":"04-run.sh", "foreground":"04-run.sh", "replace-container":"04-run.sh",
 "uninstall-service":"04-run.sh",
}
var hostActions = map[string]string{
 "install":"install.sh", "migrate":"migrate-macos.sh",
 "repair":"repair-macos.sh", "doctor":"doctor.sh",
 "start":"control.sh", "stop":"control.sh",
 "status":"control.sh", "monitor":"control.sh",
 "actions-cancel":"actions.sh", "actions-delete":"actions.sh",
 "actions-clear":"actions.sh", "cleanup":"cleanup.sh",
}

func assetsDir(executable string, cwd string) (string,error) {
 if override:=os.Getenv("GITHUB_RUNNER_CLI_ENV"); override!="" {
  if s,e:=os.Stat(override); e==nil && s.IsDir() { return override,nil }
  return "",fmt.Errorf("GITHUB_RUNNER_CLI_ENV not found: %s",override)
 }
 candidates:=[]string{filepath.Join(filepath.Dir(executable),"env","github-runner-cli"),cwd}
 for _,p:=range candidates {
  if s,e:=os.Stat(filepath.Join(p,"runner","01-download.sh")); e==nil && !s.IsDir() { return p,nil }
 }
 return "",errors.New("runtime scripts missing; run 'just install', or set GITHUB_RUNNER_CLI_ENV")
}
func execute(script string, args []string) error {
 cmd:=exec.Command("bash",append([]string{script},args...)...)
 cmd.Stdin=os.Stdin;cmd.Stdout=os.Stdout;cmd.Stderr=os.Stderr
 if err:=cmd.Run();err!=nil{return err};return nil
}
func usage() {
 fmt.Print(`github-runner-cli — GitHub Actions runner manager

Usage:
  github-runner-cli install OWNER/REPO [--allow-public] [--name NAME]
  github-runner-cli start OWNER/REPO
  github-runner-cli stop OWNER/REPO
  github-runner-cli status OWNER/REPO
  github-runner-cli monitor OWNER/REPO
  github-runner-cli refresh OWNER/REPO  # only after stopping an old container
  github-runner-cli version
  github-runner-cli runner <download|register|setup|install-service|start|stop|status|monitor|foreground|uninstall-service> OWNER/REPO [options]
  github-runner-cli runner docker-doctor
  github-runner-cli host <install|doctor|start|stop|status|monitor|repair|migrate|cleanup|actions-cancel|actions-delete|actions-clear> [args]

Environment:
  RUNNER_BACKEND=docker|macos|auto
  GITHUB_RUNNER_CLI_ENV=<path to installed runtime assets>
  RUNNER_DOCKER_SOCKET=0|1 (explicit opt-in)

The CLI preserves the existing safety checks and registered runner volumes.
`)
}
func run(args []string, executable string, cwd string) error {
 if len(args)==0 {usage();return nil}
 if args[0]=="version" {
  fmt.Println("github-runner-cli 0.2.0");return nil
 }
 if args[0]=="help" || args[0]=="--help" || args[0]=="-h" {usage();return nil}
 env,err:=assetsDir(executable,cwd);if err!=nil{return err}
 switch strings.ToLower(args[0]) {
 case "install":
  if len(args)<2 || strings.HasPrefix(args[1],"-") {return errors.New("usage: github-runner-cli install OWNER/REPO [--allow-public] [--name NAME]")}
  repo:=args[1]
  // Validate option syntax before any downloads or modifications.
  for i:=2;i<len(args);i++ {
   switch args[i] {
   case "--allow-public":
   case "--name":
    i++
    if i>=len(args) || strings.HasPrefix(args[i],"-"){return errors.New("--name requires a runner name")}
   default: return fmt.Errorf("unknown install option: %s",args[i])
   }
  }
  if err:=execute(filepath.Join(env,"runner","01-download.sh"),[]string{repo});err!=nil{return err}
  registerArgs:=append([]string{repo,"--if-needed"},args[2:]...)
  if err:=execute(filepath.Join(env,"runner","02-configure.sh"),registerArgs);err!=nil{return err}
  return execute(filepath.Join(env,"runner","04-run.sh"),[]string{repo,"install"})
 case "refresh":
  if len(args)!=2 || strings.HasPrefix(args[1],"-") {return errors.New("usage: github-runner-cli refresh OWNER/REPO")}
  return execute(filepath.Join(env,"runner","04-run.sh"),[]string{args[1],"replace-container"})
 case "start","stop","status","monitor":
  if len(args)!=2 || strings.HasPrefix(args[1],"-"){return fmt.Errorf("usage: github-runner-cli %s OWNER/REPO",strings.ToLower(args[0]))}
  return execute(filepath.Join(env,"runner","04-run.sh"),[]string{args[1],strings.ToLower(args[0])})
 case "runner":
  if len(args)<2{return errors.New("runner action required")}
  if args[1]=="docker-doctor" {
   if len(args)!=2{return errors.New("docker-doctor accepts no arguments")}
   return execute(filepath.Join(env,"runner","docker","doctor.sh"),nil)
  }
  script,ok:=runnerActions[args[1]]
  if !ok{return fmt.Errorf("unknown runner action: %s",args[1])}
  if len(args)<3{return errors.New("runner action requires OWNER/REPO")}
  extras:=args[2:]
  if args[1]=="install-service" {extras=append([]string{extras[0],"install"},extras[1:]...)}
  if args[1]=="uninstall-service" {extras=append([]string{extras[0],"uninstall"},extras[1:]...)}
  for _,action:=range []string{"start","stop","status","monitor","foreground","replace-container"} {
   if args[1]==action {extras=append([]string{extras[0],action},extras[1:]...)}
  }
  return execute(filepath.Join(env,"runner",script),extras)
 case "host":
  if len(args)<2 {return errors.New("host action required")}
  script,ok:=hostActions[args[1]]
  if !ok{return fmt.Errorf("unknown host action: %s",args[1])}
  rest:=args[2:]
  if strings.HasPrefix(args[1],"actions-") {rest=append([]string{strings.TrimPrefix(args[1],"actions-")},rest...)}
  if script=="control.sh" {rest=append([]string{args[1]},rest...)}
  if args[1]=="cleanup" && len(rest)==0 {rest=[]string{"30"}}
  return execute(filepath.Join(env,script),rest)
 default: return fmt.Errorf("unknown command: %s (try --help)",args[0])
 }
}
func main(){
 cwd,err:=os.Getwd();if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
 exe,err:=os.Executable();if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
 if err:=run(os.Args[1:],exe,cwd);err!=nil{
  fmt.Fprintln(os.Stderr,"ERROR:",err)
  var e *exec.ExitError
  if errors.As(err,&e) {os.Exit(e.ExitCode())}
  os.Exit(1)
 }
}
