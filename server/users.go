package server

import (
	"JsonAI/db"
	"JsonAI/proto"
	"context"
	"errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"log"
)

func (s Server) Login(ctx context.Context, in *proto.Login_Request) (*proto.Login_Response, error) {
	if in.Email == "" {
		return nil, status.Error(codes.InvalidArgument, "Email is required")
	}

	if in.Pin == "" {
		return nil, status.Error(codes.InvalidArgument, "Pin is required")
	}

	user, err := db.GetUserByEmail(s.DB, in.Email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "User not found")
		} else {
			log.Printf("Error in GetUserByEmail: %s", err)
			return nil, status.Error(codes.Internal, "Internal server error")
		}
	}

	if user.Pin != in.Pin {
		return nil, status.Error(codes.PermissionDenied, "Incorrect Pin")
	}

	userChatCount, err := db.GetUserChatCount(s.DB, user.UUID.ID)
	if err != nil {
		log.Printf("Error in GetUserChatCount: %s", err)
		return nil, status.Error(codes.Internal, "Internal server error")
	}

	return &proto.Login_Response{User: &proto.User{
		UserID:    user.UUID.ID,
		Name:      user.Name,
		Email:     user.Email,
		ChatCount: int32(userChatCount),
	}}, nil
}
