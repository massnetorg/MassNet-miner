PB = $(wildcard *.proto)
GO = $(PB:.proto=.pb.go)

all: $(GO)

%.pb.go: %.proto
	protoc --gogo_out=. $<
	sed 's/,omitempty//g;s/Cpu/CPU/g;s/Api/API/g;s/Grpc/GRPC/g;s/Cors/CORS/g;' config.pb.go > tmp
	mv tmp  config.pb.go
clean:
	rm *.pb.go
