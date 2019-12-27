package werft

import (
	"context"

	v1 "github.com/32leaves/werft/pkg/api/v1"
	"github.com/32leaves/werft/pkg/store"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var (
	errMissingMetadata = status.Errorf(codes.InvalidArgument, "missing token")
	errInvalidToken    = status.Errorf(codes.Unauthenticated, "invalid token")
)

// UnaryAuthInterceptor ensures that API calls are properly authenticated
func (srv *Service) UnaryAuthInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	ctx, err := validateTokenFromRequest(ctx, srv.Tokens)
	if err != nil {
		return nil, err
	}

	return handler(ctx, req)
}

// StreamAuthInterceptor ensures that API calls are properly authenticated
func (srv *Service) StreamAuthInterceptor(serv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if info.FullMethod != "/v1.WerftService/Login" {
		_, err := validateTokenFromRequest(ss.Context(), srv.Tokens)
		if err != nil {
			return err
		}
	}

	return handler(serv, ss)
}

func validateTokenFromRequest(ctx context.Context, tokens store.Token) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, errMissingMetadata
	}
	if tokens == nil {
		md["user"] = []string{"anonymous"}
		return ctx, nil
	}

	tkn := md["authorization"]
	if len(tkn) == 0 {
		return nil, errMissingMetadata
	}

	user, err := tokens.Get(tkn[0])
	if err == store.ErrNotFound {
		return nil, errInvalidToken
	}
	if err != nil {
		log.WithError(err).Error("cannot validate auth token")
		return nil, status.Errorf(codes.Internal, "cannot validate auth token")
	}
	md["user"] = []string{user}
	ctx = metadata.NewIncomingContext(ctx, md)

	return ctx, nil
}

// AuthProvider can authenticate users
type AuthProvider interface {
	Login() (<-chan *v1.LoginResponse, <-chan error)
}

// AddAuthProvider makes an auth provider available for login
func (srv *Service) AddAuthProvider(name string, p AuthProvider, makeDefault bool) {
	if makeDefault || len(srv.authProvider) == 0 {
		srv.defaultAuthProvider = p
	}

	srv.authProvider[name] = p
}
