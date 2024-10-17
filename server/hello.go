package server

import (
	"JsonAI/proto"
	"context"
)

func (s Server) SayHello(ctx context.Context, in *proto.SayHello_Request) (*proto.SayHello_Response, error) {
	return &proto.SayHello_Response{Message: "Hello " + in.Name}, nil
}
