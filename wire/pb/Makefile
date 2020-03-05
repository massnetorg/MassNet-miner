PB = $(wildcard *.proto)
GO = $(PB:.proto=.pb.go)
WIREPB_PB_GO = wirepb.pb.go

all: $(GO)

%.pb.go: %.proto
	protoc --gofast_out=. $<
	sed -i.bak 's/,omitempty//;s/ChainId/ChainID/' $(WIREPB_PB_GO)
	rm $(WIREPB_PB_GO).bak
clean:
	rm *.pb.go
