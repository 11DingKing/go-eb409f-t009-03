# Bug Reproduction

## 包的性质

当前 test_model_fix 保存的是被测模型修复后的结果源码，不是初始含 Bug 源码。要复现原始缺陷，必须检出下面固定的 parent SHA；不要在当前修复结果源码上期待重新出现修复前失败。生成系统使用的可信验证补丁和完整验证日志仅在本地留存，不提交到结果分支。

## 问题现象

黑启动执行中途取消不掉，请帮我修复。

go test ./... 里这条一直是红的：

  --- FAIL: TestBlackStart_CancelledExecution (0.40s)
      service_test.go:500: expected cancellation error

对应的现场是：值班员发起黑启动执行后中止了这次操作（调用方把传进去的 context 取消掉），执行序列却一路跑到底，把参与的储能舱全部带电，黑启动状态直接变成 completed，返回也没有任何错误，审计里看不到 cancelled 记录。黑启动是全站恢复操作，中途必须能停下来，否则一次误操作没法撤回。

期望：调用方取消 context 之后，执行序列要停下来、把这次黑启动置为失败并返回取消错误；未取消时的正常执行行为保持不变。修复后请保证 go test ./... 全绿，不要修改或跳过测试。

## 含 Bug 版本

- 仓库：11DingKing/go-eb409f-t009-03
- 仓库地址：https://github.com/11DingKing/go-eb409f-t009-03.git
- parent SHA：e8432fcb0b6715f825e1808623fe6cc5f3708d83

## 复现步骤

```bash
git clone -- https://github.com/11DingKing/go-eb409f-t009-03.git bug-repro
cd bug-repro
git checkout --detach e8432fcb0b6715f825e1808623fe6cc5f3708d83
go test ./... -run "^TestBlackStart_CancelledExecution$" -count=1 -v
```

## 双架构完整错误信息

### linux/amd64

- 容器内复现预期退出码：1
- 容器内复现实际退出码：1

stdout：

```text
$ go test ./... -run "^TestBlackStart_CancelledExecution$" -count=1 -v
?   	github.com/ejinagrid/ejinagrid/cmd/ejinagrid	[no test files]
testing: warning: no tests to run
PASS
ok  	github.com/ejinagrid/ejinagrid/internal/api	0.113s [no tests to run]
?   	github.com/ejinagrid/ejinagrid/internal/clock	[no test files]
testing: warning: no tests to run
PASS
ok  	github.com/ejinagrid/ejinagrid/internal/domain	0.088s [no tests to run]
=== RUN   TestBlackStart_CancelledExecution
    service_test.go:500: expected cancellation error
--- FAIL: TestBlackStart_CancelledExecution (0.41s)
FAIL
FAIL	github.com/ejinagrid/ejinagrid/internal/service	0.494s
testing: warning: no tests to run
PASS
ok  	github.com/ejinagrid/ejinagrid/internal/store	0.086s [no tests to run]
testing: warning: no tests to run
PASS
ok  	github.com/ejinagrid/ejinagrid/internal/worker	0.091s [no tests to run]
FAIL

```

stderr：

```text
(empty)
```

### linux/arm64

- 容器内复现预期退出码：1
- 容器内复现实际退出码：1

stdout：

```text
$ go test ./... -run "^TestBlackStart_CancelledExecution$" -count=1 -v
?   	github.com/ejinagrid/ejinagrid/cmd/ejinagrid	[no test files]
testing: warning: no tests to run
PASS
ok  	github.com/ejinagrid/ejinagrid/internal/api	0.003s [no tests to run]
?   	github.com/ejinagrid/ejinagrid/internal/clock	[no test files]
testing: warning: no tests to run
PASS
ok  	github.com/ejinagrid/ejinagrid/internal/domain	0.002s [no tests to run]
=== RUN   TestBlackStart_CancelledExecution
    service_test.go:500: expected cancellation error
--- FAIL: TestBlackStart_CancelledExecution (0.40s)
FAIL
FAIL	github.com/ejinagrid/ejinagrid/internal/service	0.406s
testing: warning: no tests to run
PASS
ok  	github.com/ejinagrid/ejinagrid/internal/store	0.002s [no tests to run]
testing: warning: no tests to run
PASS
ok  	github.com/ejinagrid/ejinagrid/internal/worker	0.002s [no tests to run]
FAIL

```

stderr：

```text
(empty)
```

## 通过条件

定向测试通过：go test ./... -run '^TestBlackStart_CancelledExecution$' -count=1 -v
全量回归 go test -timeout=120s -count=1 ./... 通过，go build ./... 与 go vet ./... 通过
取消 context 后 ExecuteBlackStart 返回非 nil 取消错误且黑启动状态为 failed；未取消时仍把全部健康舱带电并置 completed；不得修改或跳过既有测试
