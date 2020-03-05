PB = $(wildcard *.proto)
GO = $(PB:.proto=.pb.go)
API_PB_GO = api.pb.go

all: $(GO)

%.pb.go: %.proto
	protoc -I/usr/local/include -I. -I${GOPATH}/src \
	-I${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
	-I ${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway \
	--gogo_out=plugins=grpc:. \
	--grpc-gateway_out=logtostderr=true:. $< \
	--swagger_out=logtostderr=true:.
	sed -i.bak 's/Size_/Size/' $(API_PB_GO)
	rm $(API_PB_GO).bak

doc:
	protoc -I/usr/local/include -I. -I${GOPATH}/src \
		-I${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
		-I ${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway \
  		--doc_out=${GOPATH}/src/massnet.org/mass/docs \
  		--doc_opt=html,API_swagger.html \
  		${GOPATH}/src/massnet.org/mass/api/proto/api.proto

clean:
	rm *.go
	rm *.swagger.json