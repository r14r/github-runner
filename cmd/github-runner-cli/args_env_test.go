package main

import "testing"

func TestRunnerRepoEnvironmentFallback(t *testing.T) {
 t.Setenv("RUNNER_REPO", "r14r/BookBuilder")
 repo, rest, err := requireRepo(nil)
 if err != nil || repo != "r14r/BookBuilder" || len(rest)!=0 {
  t.Fatalf("fallback: repo=%q rest=%v err=%v", repo,rest,err)
 }
 repo,_,err=requireRepo([]string{"--repo","r14r/github-runner"})
 if err!=nil || repo!="r14r/github-runner" {t.Fatalf("flag precedence: repo=%q err=%v",repo,err)}
 repo,_,err=repoOption(nil)
 if err!=nil || repo!="r14r/BookBuilder" {t.Fatalf("optional fallback: repo=%q err=%v",repo,err)}
}

func TestRunnerRepoEnvironmentInvalid(t *testing.T) {
 t.Setenv("RUNNER_REPO","invalid")
 _,_,err:=requireRepo(nil)
 if err==nil {t.Fatal("invalid RUNNER_REPO should fail")}
 repo,_,err:=requireRepo([]string{"--repo","r14r/github-runner"})
 if err!=nil || repo!="r14r/github-runner" {t.Fatalf("explicit override: repo=%q err=%v",repo,err)}
}

func TestRunnerRepoEnvironmentAbsent(t *testing.T) {
 t.Setenv("RUNNER_REPO","")
 repo,rest,err:=repoOption(nil)
 if err!=nil || repo!="" || len(rest)!=0 {t.Fatalf("optional: repo=%q rest=%v err=%v",repo,rest,err)}
 if _,_,err:=requireRepo(nil);err==nil {t.Fatal("repository should be required")}
}
