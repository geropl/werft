package werft

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var (
	errMissingMetadata = status.Errorf(codes.InvalidArgument, "missing metadata")
	errInvalidToken    = status.Errorf(codes.Unauthenticated, "invalid token")
)

// UnaryAuthInterceptor ensures that API calls are properly authenticated
func (srv *Service) UnaryAuthInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// authentication (token verification)
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, errMissingMetadata
	}

	tkn := md["authorization"]
	if len(tkn) == 0 || tkn[0] != "foobar" {
		return nil, errInvalidToken
	}
	md["user"] = []string{"foobar"}
	ctx = metadata.NewIncomingContext(ctx, md)

	return handler(ctx, req)
}
