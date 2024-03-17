run:
	go run example/*.go

build-run:
	go build -ldflags "-s -w" -gcflags "-N -l" -o main example/*.go
	./main

gc-trace-run:
	GODEBUG=gctrace=1 go run example/*.go

test:
	go clean -testcache && go test .

test-cover:
	go test -race -coverprofile=coverage.txt -covermode=atomic
	go tool cover -html=coverage.txt -o coverage.html
	rm coverage.txt

bench:
	go test -bench .

gen-proto:
	rm -rf proto && protoc --go_out=. --go_opt=Mcache.proto=proto/ cache.proto

# pprof relatived.
# cpu analysis.
web-profile:
	go tool pprof -http=:18081 "http://localhost:6060/debug/pprof/profile?seconds=60"

# mem analysis.
cmd-heap:
	go tool pprof "http://localhost:6060/debug/pprof/heap"
web-heap:
	go tool pprof -http=:18082 "http://localhost:6060/debug/pprof/heap?seconds=60"

cmd-allocs:
	go tool pprof "http://localhost:6060/debug/pprof/allocs"
web-allocs:
	go tool pprof -http=:18083 "http://localhost:6060/debug/pprof/allocs?seconds=60"

# heap object analysis.
cmd-mutex:
	go tool pprof "http://localhost:6060/debug/pprof/mutex"
cmd-block:
	go tool pprof "http://localhost:6060/debug/pprof/block"