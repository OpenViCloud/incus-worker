package provider

import (
	"context"
	"fmt"

	"github.com/lxc/incus/v7/shared/api"
)

// waitSeconds bounds one server-side wait so a cancelled context is noticed between waits.
const waitSeconds = 30

// OperationWaiter is the one call WaitOperation needs; incus.InstanceServer has it.
type OperationWaiter interface {
	GetOperationWait(uuid string, timeout int) (*api.Operation, string, error)
}

// WaitOperation polls /operations/{id}/wait until the operation is final. A 404 means Incus forgot the operation (it
// keeps finished ones for seconds, lab 09): the outcome is unknown, which is an error, not a success.
func WaitOperation(ctx context.Context, waiter OperationWaiter, operationId string) error {
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("waiting for incus operation %s: %w", operationId, err)
		}
		operation, _, err := waiter.GetOperationWait(operationId, waitSeconds)
		if err != nil {
			return fmt.Errorf("waiting for incus operation %s: %w", operationId, err)
		}
		if !operation.StatusCode.IsFinal() {
			continue
		}
		if operation.StatusCode == api.Success {
			return nil
		}
		return &OperationFailedError{Description: operation.Description, Status: operation.Status, Message: operation.Err}
	}
}
