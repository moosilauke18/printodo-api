all: 
	go build && ./printodo-api

docker: docker-build docker-push

docker-build:
	docker build -t evandev/printodo-api .

docker-push:
	docker push evandev/printodo-api
