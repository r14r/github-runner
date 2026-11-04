package monitor

import (
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

func TestMonitorSnapshotAndIsolation(t *testing.T) {
 s:=New([]Runner{{Repository:"example/demo",Container:"runner-example"}})
 for _,path:=range []string{"/api/monitor","/api/monitor/runners"} {
  w:=httptest.NewRecorder()
  s.Handler().ServeHTTP(w,httptest.NewRequest(http.MethodGet,path,nil))
  if w.Code!=200 {t.Fatalf("%s status %d",path,w.Code)}
  if !json.Valid(w.Body.Bytes()) {t.Fatalf("%s invalid json %s",path,w.Body.String())}
 }
 w:=httptest.NewRecorder()
 s.Handler().ServeHTTP(w,httptest.NewRequest(http.MethodGet,"/api/monitor/runners/not-installed/events",nil))
 if w.Code!=404 {t.Fatalf("expected 404, got %d",w.Code)}
 w=httptest.NewRecorder()
 s.Handler().ServeHTTP(w,httptest.NewRequest(http.MethodGet,"/api/monitor/runners/runner-example/events?tail=1001",nil))
 if w.Code!=400 {t.Fatalf("expected invalid tail 400, got %d",w.Code)}
 if err:=s.Stream(httptest.NewRequest(http.MethodGet,"/",nil).Context(),"untrusted-container",10,func(Event){});err==nil||!strings.Contains(err.Error(),"unknown"){t.Fatalf("unknown container should be rejected: %v",err)}
}
