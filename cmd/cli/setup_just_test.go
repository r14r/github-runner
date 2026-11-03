package main

import (
 "strings"
 "testing"
)

func TestSetupJustRecognizedAsWorkflowManaged(t *testing.T) {
 for _, workflow := range []string{
  "jobs:\n  build:\n    runs-on: ubuntu-24.04\n    steps:\n      - uses: extractions/setup-just@v2\n      - run: just ci\n",
  "jobs:\n  build:\n    runs-on: ubuntu-24.04\n    steps:\n      - uses: extractions/setup-just@v4\n        with:\n          just-version: '1.58.0'\n      - run: just test\n",
 } {
  tools:=map[string]toolRequirement{}
  actions:=map[string]bool{}
  var warnings []string
  analyzeWorkflowText(".github/workflows/ci.yml",workflow,tools,actions,&warnings)
  if len(warnings)!=0 { t.Fatalf("unexpected warnings: %v", warnings) }
  req,ok:=tools["just"]
  if !ok || !toolManagedByWorkflow(req) {t.Fatalf("just not managed by workflow: %#v",req)}
  if !strings.Contains(req.Source,"extractions/setup-just@") {t.Fatalf("unexpected source: %s",req.Source)}
  if _,err:=toolchainBuildArgs(toolchainManifest{Profile:"toolchain",Tools:[]toolRequirement{req}});err!=nil {t.Fatalf("build rejected setup-just: %v",err)}
  if strings.Contains(workflow,"just-version:") && req.Version!="1.58.0" {t.Fatalf("just-version ignored: %q",req.Version)}
 }
}
