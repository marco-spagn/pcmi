package grpcserver

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRequireWriteRole(t *testing.T) {
	if err := requireWriteRole("admin"); err != nil {
		t.Fatal(err)
	}
	if err := requireWriteRole("user"); err != nil {
		t.Fatal(err)
	}
	err := requireWriteRole("readonly")
	if err == nil {
		t.Fatal("readonly must be denied")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.PermissionDenied {
		t.Fatalf("got %v", err)
	}
}

func TestMapSvcValidationErr(t *testing.T) {
	st, ok := status.FromError(mapSvcValidationErr("ctx", fmt.Errorf("items required")))
	if !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument, got %v", st)
	}
	st2, ok := status.FromError(mapSvcValidationErr("ctx", fmt.Errorf("maximum 50 items")))
	if !ok || st2.Code() != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument for maximum, got %v", st2)
	}
	st3, ok := status.FromError(mapSvcValidationErr("ctx", errors.New("db down")))
	if !ok || st3.Code() != codes.Internal {
		t.Fatalf("want Internal, got %v", st3)
	}
}
