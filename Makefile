SRC_FILES= request.go\
	  workunit.go \
	  responder.go \
	  filters.go \
	  artifacts.go \
	  main.go
TEST_FILES= workunit_test.go \
	    filters_test.go \
	    artifacts_test.go \
	    test_scaffold.go \
	    request_test.go \
	    main_test.go


BINARY=catalog_worker
.DEFAULT_GOAL := build

build:
	go build -o ${BINARY} ${SRC_FILES}

test:
	go test -v ${TEST_FILES} ${SRC_FILES}

test_debug:
	dlv test ${TEST_FILES} ${SRC_FILES}

coverage:
	rm -rf coverage.out
	go test -coverprofile=coverage.out ${TEST_FILES} ${SRC_FILES}
	go tool cover -html=coverage.out

format:
	go fmt ${SRC_FILES} ${TEST_FILES}

run:
	go run ${SRC_FILES}

race:
	go run -race ${SRC_FILES}

debug:
	dlv debug ${SRC_FILES}

linux: 
	GOOS=linux GOARCH=arm go build -x -o catalog_worker.linux ${SRC_FILES}

clean:
	go clean

lint:
	golint ${SRC_FILES} ${TEST_FILES}
