gc-trace-run:
	GODEBUG=gctrace=1 go run example/*.go

test-cover:
	go test -race -coverprofile=coverage.txt -covermode=atomic
	go tool cover -html=coverage.txt -o coverage.html
	rm coverage.txt

bench:
	go test -bench . -benchmem

web-profile:
	go tool pprof -http=:18081 "http://localhost:6060/debug/pprof/profile?seconds=30"

cmd-heap:
	go tool pprof "http://localhost:6060/debug/pprof/heap"

web-heap:
	go tool pprof -http=:18082 "http://localhost:6060/debug/pprof/heap?seconds=60"

cmd-allocs:
	go tool pprof "http://localhost:6060/debug/pprof/allocs"

web-allocs:
	go tool pprof -http=:18083 "http://localhost:6060/debug/pprof/allocs?seconds=60"

cmd-mutex:
	go tool pprof "http://localhost:6060/debug/pprof/mutex"

cmd-block:
	go tool pprof "http://localhost:6060/debug/pprof/block"