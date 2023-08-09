run:
	go run example/*.go

build-run:
	go build -ldflags "-s -w" -gcflags "-N -l" -o main example/*.go
	./main

gc-trace-run:
	GODEBUG=gctrace=1 go run example/*.go

test:
	go clean -testcache && go test .

# 生成 html 覆盖率文件
test-cover:
	go test -coverprofile=coverage.out
	go tool cover -html=coverage.out

bench:
	go test -bench .

pprof:
	go tool pprof -http=:18081 "http://localhost:6060/debug/pprof/profile?seconds=60"