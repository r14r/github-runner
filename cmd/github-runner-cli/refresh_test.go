package main

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func fakeRefreshRuntime(r dockerRunner, present, running, busy bool, events *[]string) refreshRuntime {
	add := func(parts ...string) { *events = append(*events, strings.Join(parts, " ")) }
	return refreshRuntime{
		exists: func(kind, name string) bool {
			if kind != "container" || name != r.Container { panic("unexpected container lookup") }
			return present
		},
		inspect: func(format string) (string, error) {
			if format == "{{.State.Running}}" {
				if running { return "true", nil }
				return "false", nil
			}
			if strings.Contains(format, "/var/run/docker.sock") { return "", nil }
			return "", fmt.Errorf("unexpected docker inspect: %s", format)
		},
		local: func() (localRunner, error) {
			add("local")
			return localRunner{Repository:r.Repo, Identity:runnerIdentity{AgentID:42,AgentName:"runner-1"}},nil
		},
		remote: func() ([]runnerRecord, error) {
			add("remote")
			return []runnerRecord{{ID:42,Name:"runner-1",Status:"online",Busy:busy}},nil
		},
		docker:func(args ...string)error { add(args...); return nil },
		create:func()error { add("create"); return nil },
	}
}
func TestRefreshRunningIdleStopsRecreatesAndRestarts(t *testing.T) {
	t.Setenv("RUNNER_DOCKER_SOCKET","0")
	r:=dockerRunner{Repo:"example-org/demo",Container:"github-runner-example-org-demo",Image:"generic:test",Volume:"runner-state"}
	events:=[]string{}
	rt:=fakeRefreshRuntime(r,true,true,false,&events)
	if err:=r.refreshWithRuntime(rt);err!=nil{t.Fatal(err)}
	expected:=[]string{"local","remote","stop "+r.Container,"rm "+r.Container,"create","start "+r.Container}
	if !reflect.DeepEqual(events,expected){t.Fatalf("actual %v, expected %v",events,expected)}
}
func TestRefreshStoppedDoesNotStartOrCheckGitHub(t *testing.T){
	t.Setenv("RUNNER_DOCKER_SOCKET","0")
	r:=dockerRunner{Repo:"example-org/demo",Container:"runner-test",Volume:"registration"}
	events:=[]string{}
	rt:=fakeRefreshRuntime(r,true,false,false,&events)
	if err:=r.refreshWithRuntime(rt);err!=nil{t.Fatal(err)}
	if !reflect.DeepEqual(events,[]string{"rm runner-test","create"}){t.Fatal(events)}
}
func TestRefreshMissingCreatesStopped(t *testing.T){
	r:=dockerRunner{Repo:"example-org/demo",Container:"runner-test"}
	events:=[]string{}
	rt:=fakeRefreshRuntime(r,false,false,false,&events)
	if err:=r.refreshWithRuntime(rt);err!=nil{t.Fatal(err)}
	if !reflect.DeepEqual(events,[]string{"create"}){t.Fatal(events)}
}
func TestRefreshBusyRefusesWithoutDockerChanges(t *testing.T){
	t.Setenv("RUNNER_DOCKER_SOCKET","0")
	r:=dockerRunner{Repo:"example-org/demo",Container:"runner-test"}
	events:=[]string{}
	rt:=fakeRefreshRuntime(r,true,true,true,&events)
	err:=r.refreshWithRuntime(rt)
	if err==nil||!strings.Contains(err.Error(),"busy"){t.Fatalf("expected busy error, got %v",err)}
	if !reflect.DeepEqual(events,[]string{"local","remote"}){t.Fatal(events)}
}
func TestRefreshGitHubErrorsFailClosed(t *testing.T){
	r:=dockerRunner{Repo:"example-org/demo",Container:"runner-test"}
	for _,tc:=range []struct{name string; change func(*refreshRuntime)}{
		{"offline API",func(x *refreshRuntime){x.remote=func()([]runnerRecord,error){return nil,errors.New("gh auth expired")}}},
		{"invalid identity",func(x *refreshRuntime){x.local=func()(localRunner,error){return localRunner{Repository:r.Repo,Identity:runnerIdentity{AgentName:"runner-1"}},nil}}},
		{"identity mismatch",func(x *refreshRuntime){x.remote=func()([]runnerRecord,error){return []runnerRecord{{ID:100,Name:"runner-1"}},nil}}},
		{"unregistered runner",func(x *refreshRuntime){x.remote=func()([]runnerRecord,error){return nil,nil}}},
		{"repository mismatch",func(x *refreshRuntime){x.local=func()(localRunner,error){return localRunner{Repository:"other/example",Identity:runnerIdentity{AgentID:42,AgentName:"runner-1"}},nil}}},
	} {
		t.Run(tc.name,func(t *testing.T){
			events:=[]string{}
			rt:=fakeRefreshRuntime(r,true,true,false,&events)
			tc.change(&rt)
			err:=r.refreshWithRuntime(rt)
			if err==nil{t.Fatal("unsafe refresh accepted")}
			for _,ev:=range events{if strings.HasPrefix(ev,"stop ")||strings.HasPrefix(ev,"rm ")||ev=="create"||strings.HasPrefix(ev,"start "){t.Fatalf("changed Docker state on remote error: %v",events)}}
		})
	}
}
func TestRefreshSocketMountRequiresExplicitOptInBeforeStop(t *testing.T){
	t.Setenv("RUNNER_DOCKER_SOCKET","0")
	r:=dockerRunner{Repo:"example-org/demo",Container:"runner-test"}
	events:=[]string{}
	rt:=fakeRefreshRuntime(r,true,true,false,&events)
	old:=rt.inspect
	rt.inspect=func(format string)(string,error){
		if strings.Contains(format,"/var/run/docker.sock"){return "yes",nil}
		return old(format)
	}
	err:=r.refreshWithRuntime(rt)
	if err==nil||!strings.Contains(err.Error(),"RUNNER_DOCKER_SOCKET=1"){t.Fatalf("unexpected %v",err)}
	if len(events)!=0{t.Fatalf("preflight changed state: %v",events)}
}
func TestRefreshStopFailureDoesNotRemove(t *testing.T){
	r:=dockerRunner{Repo:"example-org/demo",Container:"runner-test"}
	events:=[]string{}
	rt:=fakeRefreshRuntime(r,true,true,false,&events)
	rt.docker=func(args ...string)error{
		events=append(events,strings.Join(args," "))
		if args[0]=="stop"{return errors.New("cannot stop")}
		return nil
	}
	err:=r.refreshWithRuntime(rt)
	if err==nil||!strings.Contains(err.Error(),"cannot stop"){t.Fatal(err)}
	if len(events)!=3||!strings.HasPrefix(events[2],"stop "){t.Fatal(events)}
}
func TestRefreshRemoveFailureRecoversOldContainer(t *testing.T){
	r:=dockerRunner{Repo:"example-org/demo",Container:"runner-test"}
	events:=[]string{}
	rt:=fakeRefreshRuntime(r,true,true,false,&events)
	rt.docker=func(args ...string)error{
		events=append(events,strings.Join(args," "))
		if args[0]=="rm"{return errors.New("remove failed")}
		return nil
	}
	err:=r.refreshWithRuntime(rt)
	if err==nil||!strings.Contains(err.Error(),"remove failed"){t.Fatal(err)}
	expected:=[]string{"local","remote","stop runner-test","rm runner-test","start runner-test"}
	if !reflect.DeepEqual(events,expected){t.Fatal(events)}
}
func TestRefreshCreateFailurePreservesVolumeAndDoesNotRestart(t *testing.T){
	r:=dockerRunner{Repo:"example-org/demo",Container:"runner-test",Volume:"runner-state"}
	events:=[]string{}
	rt:=fakeRefreshRuntime(r,true,true,false,&events)
	rt.create=func()error{events=append(events,"create");return errors.New("disk full")}
	err:=r.refreshWithRuntime(rt)
	if err==nil||!strings.Contains(err.Error(),"runner-state preserved"){t.Fatal(err)}
	if events[len(events)-1]!="create"{t.Fatalf("unexpected restart: %v",events)}
}
func TestRefreshRestartsOnlyWhenPreviouslyRunning(t *testing.T){
	r:=dockerRunner{Repo:"example-org/demo",Container:"runner-test"}
	events:=[]string{}
	rt:=fakeRefreshRuntime(r,true,true,false,&events)
	rt.docker=func(args ...string)error{
		events=append(events,strings.Join(args," "))
		if args[0]=="start"{return errors.New("start refused")}
		return nil
	}
	err:=r.refreshWithRuntime(rt)
	if err==nil||!strings.Contains(err.Error(),"could not restart"){t.Fatal(err)}
	if events[len(events)-1]!="start runner-test"{t.Fatal(events)}
}
