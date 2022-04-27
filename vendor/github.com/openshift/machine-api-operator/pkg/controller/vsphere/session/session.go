/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package session

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"

	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/google/uuid"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/soap"
	"k8s.io/klog/v2"
)

var sessionCache = map[string]Session{}
var sessionMU sync.Mutex

const (
	managedObjectTypeTask = "Task"
)

// Session is a vSphere session with a configured Finder.
type Session struct {
	*govmomi.Client
	Finder     *find.Finder
	Datacenter *object.Datacenter

	username string
	password string
}

// GetOrCreate gets a cached session or creates a new one if one does not
// already exist.
func GetOrCreate(
	ctx context.Context,
	server, datacenter, username, password string, insecure bool) (*Session, error) {

	sessionMU.Lock()
	defer sessionMU.Unlock()

	sessionKey := server + username + datacenter
	if session, ok := sessionCache[sessionKey]; ok {
		sessionActive, err := session.SessionManager.SessionIsActive(ctx)
		if err != nil {
			klog.Errorf("Error performing session check request to vSphere: %v", err)
		}
		if sessionActive {
			return &session, nil
		}
	}
	klog.Infof("No existing vCenter session found, creating new session")

	soapURL, err := soap.ParseURL(server)
	if err != nil {
		return nil, fmt.Errorf("error parsing vSphere URL %q: %w", server, err)
	}
	if soapURL == nil {
		return nil, fmt.Errorf("error parsing vSphere URL %q", server)
	}

	// Set user to nil there for prevent login during client creation.
	// See https://github.com/vmware/govmomi/blob/master/client.go#L91
	soapURL.User = nil
	client, err := govmomi.NewClient(ctx, soapURL, insecure)
	if err != nil {
		return nil, fmt.Errorf("error setting up new vSphere SOAP client: %w", err)
	}

	// Set up user agent before login for being able to track mapi component in vcenter sessions list
	client.UserAgent = "machineAPIvSphereProvider"
	if err := client.Login(ctx, url.UserPassword(username, password)); err != nil {
		return nil, fmt.Errorf("unable to login to vCenter: %w", err)
	}

	session := Session{
		Client:   client,
		username: username,
		password: password,
	}

	session.Finder = find.NewFinder(session.Client.Client, false)

	dc, err := session.Finder.DatacenterOrDefault(ctx, datacenter)
	if err != nil {
		return nil, fmt.Errorf("unable to find datacenter %q: %w", datacenter, err)
	}
	session.Datacenter = dc
	session.Finder.SetDatacenter(dc)

	// Cache the session.
	sessionCache[sessionKey] = session

	return &session, nil
}

func (s *Session) FindVM(ctx context.Context, UUID, name string) (*object.VirtualMachine, error) {
	if !isValidUUID(UUID) {
		klog.V(3).Infof("Invalid UUID for VM %q: %s, trying to find by name", name, UUID)
		return s.findVMByName(ctx, name)
	}
	klog.V(3).Infof("Find template by instance uuid: %s", UUID)
	ref, err := s.FindRefByInstanceUUID(ctx, UUID)
	if ref != nil && err == nil {
		return object.NewVirtualMachine(s.Client.Client, ref.Reference()), nil
	}
	if err != nil {
		klog.V(3).Infof("Instance not found by UUID: %s, trying to find by name %q", err, name)
	}
	return s.findVMByName(ctx, name)
}

// FindByInstanceUUID finds an object by its instance UUID.
func (s *Session) FindRefByInstanceUUID(ctx context.Context, UUID string) (object.Reference, error) {
	return s.findRefByUUID(ctx, UUID, true)
}

func (s *Session) findRefByUUID(ctx context.Context, UUID string, findByInstanceUUID bool) (object.Reference, error) {
	if s.Client == nil {
		return nil, errors.New("vSphere client is not initialized")
	}
	si := object.NewSearchIndex(s.Client.Client)
	ref, err := si.FindByUuid(ctx, s.Datacenter, UUID, true, &findByInstanceUUID)
	if err != nil {
		return nil, fmt.Errorf("error finding object by uuid %q: %w", UUID, err)
	}
	return ref, nil
}

func (s *Session) findVMByName(ctx context.Context, ID string) (*object.VirtualMachine, error) {
	tpl, err := s.Finder.VirtualMachine(ctx, ID)
	if err != nil {
		if isNotFound(err) {
			return nil, err
		}
		return nil, fmt.Errorf("unable to find template by name %q: %w", ID, err)
	}
	return tpl, nil
}

func isNotFound(err error) bool {
	switch err.(type) {
	case *find.NotFoundError:
		return true
	default:
		return false
	}
}

func isValidUUID(str string) bool {
	_, err := uuid.Parse(str)
	return err == nil
}

func (s *Session) GetTask(ctx context.Context, taskRef string) (*mo.Task, error) {
	if taskRef == "" {
		return nil, errors.New("taskRef can't be empty")
	}
	var obj mo.Task
	moRef := types.ManagedObjectReference{
		Type:  managedObjectTypeTask,
		Value: taskRef,
	}
	if err := s.RetrieveOne(ctx, moRef, []string{"info"}, &obj); err != nil {
		return nil, err
	}
	return &obj, nil
}

func (s *Session) WithRestClient(ctx context.Context, f func(c *rest.Client) error) error {
	c := rest.NewClient(s.Client.Client)

	user := url.UserPassword(s.username, s.password)
	if err := c.Login(ctx, user); err != nil {
		return err
	}

	defer func() {
		if err := c.Logout(ctx); err != nil {
			klog.Errorf("Failed to logout: %v", err)
		}
	}()

	return f(c)
}
