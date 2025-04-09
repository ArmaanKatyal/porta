run: build
	./bin/server

build:
	go build -o bin/server ./cmd

clean:
	rm -rf bin

lint:
	golangci-lint run

tidy:
	go mod tidy

client: build_client
	./bin/client --port 3000
client2:
	./bin/client --port 3001

build_client:
	go build -o bin/client ./client

image:
	docker build -t go-api-gateway .

linux_image:
	docker buildx build --platform linux/amd64 . -t armaan24katyal/go-api-gateway:latest

k8:
	kubectl apply -f ./porta

run_image:
	docker run --env=GOPATH=/go --network=bridge --workdir=/app -p 8080:8080 --restart=no --runtime=runc --name gateway -d go-api-gateway:latest

dclean:
	docker stop gateway
	docker rm -f gateway
	docker rmi go-api-gateway:latest

test:
	go test -v ./...
