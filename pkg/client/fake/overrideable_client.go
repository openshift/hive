package fake

// DNM

import (
	"context"

	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// FakeClientWithCustomErrors overrides some of the fake client's methods, allowing them to (not
// actually run and) throw specific errors. Use it like this:
//
//	realFakeClient := NewFakeClient(...)
//	c := FakeClientWithCustomErrors{
//	    Client: realFakeClient,
//	    DeleteBehavior: []error{nil, fmt.Errorf("Error on the second call"), nil}
//	    UpdateBehavior: []error(fmt.Errorf("Error on the first call"))
//	}
//	c.Delete(...) // runs the real fake Delete
//	c.Delete(...) // skips the real fake Delete and returns the first error
//	c.Delete(...) // runs the real fake Delete
//	c.Delete(...) // runs the real fake Delete, even though we ran off the end of the array
//	c.Update(...) // returns the error
//	c.Get(...)    // runs the real fake Get, because no overrides
type FakeClientWithCustomErrors struct {
	// The "Real" fake client
	crclient.Client
	// Entries in this list affect each successive call to Get(). Using `nil` causes the "real"
	// Get to be called. Using a non-nil error causes the "real" Get to be skipped, and that
	// error to be returned instead.
	GetBehavior []error
	// Private tracker of calls to Get, used to determine which GetBehavior to use.
	numGetCalls int
	// Ditto List
	ListBehavior []error
	// Private tracker of calls to List, used to determine which ListBehavior to use.
	numListCalls int
	// Ditto Delete
	DeleteBehavior []error
	// Private tracker of calls to Delete, used to determine which DeleteBehavior to use.
	numDeleteCalls int
	// Ditto Update
	UpdateBehavior []error
	// Private tracker of calls to Update, used to determine which UpdateBehavior to use.
	numUpdateCalls int
	// TODO(efried): Add the other methods. Propose upstream.
}

func clientOverride(behavior []error, numCalls int) error {
	if len(behavior) > numCalls {
		return behavior[numCalls] // which might be nil
	}
	// If we ran off the end, assume default behavior
	return nil
}

// Get overrides the fake client's Get, conditionally bypassing it and returning an error instead.
func (f FakeClientWithCustomErrors) Get(ctx context.Context, key crclient.ObjectKey, obj crclient.Object, opts ...crclient.GetOption) error {
	// Always increment the call count, but not until we're done.
	defer func() { f.numGetCalls++ }()
	if err := clientOverride(f.GetBehavior, f.numGetCalls); err != nil {
		return err
	}
	// Otherwise run the real Get
	return f.Client.Get(ctx, key, obj, opts...)
}

// List overrides the fake client's List, conditionally bypassing it and returning an error instead.
func (f FakeClientWithCustomErrors) List(ctx context.Context, list crclient.ObjectList, opts ...crclient.ListOption) error {
	// Always increment the call count, but not until we're done.
	defer func() { f.numListCalls++ }()
	if err := clientOverride(f.ListBehavior, f.numListCalls); err != nil {
		return err
	}
	// Otherwise run the real List
	return f.Client.List(ctx, list, opts...)
}

// Delete overrides the fake client's Delete, conditionally bypassing it and returning an error instead.
func (f FakeClientWithCustomErrors) Delete(ctx context.Context, obj crclient.Object, opts ...crclient.DeleteOption) error {
	// Always increment the call count, but not until we're done.
	defer func() { f.numDeleteCalls++ }()
	if err := clientOverride(f.DeleteBehavior, f.numDeleteCalls); err != nil {
		return err
	}
	// Otherwise run the real Delete
	return f.Client.Delete(ctx, obj, opts...)
}

// Update overrides the fake client's Update, conditionally bypassing it and returning an error instead.
func (f FakeClientWithCustomErrors) Update(ctx context.Context, obj crclient.Object, opts ...crclient.UpdateOption) error {
	// Always increment the call count, but not until we're done.
	defer func() { f.numUpdateCalls++ }()
	if err := clientOverride(f.UpdateBehavior, f.numUpdateCalls); err != nil {
		return err
	}
	// Otherwise run the real Update
	return f.Client.Update(ctx, obj, opts...)
}
