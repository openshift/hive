// Code generated by go-swagger; DO NOT EDIT.

package internal_operations_snapshots

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"

	"github.com/IBM-Cloud/power-go-client/power/models"
)

// InternalV1OperationsSnapshotsDeleteReader is a Reader for the InternalV1OperationsSnapshotsDelete structure.
type InternalV1OperationsSnapshotsDeleteReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *InternalV1OperationsSnapshotsDeleteReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 204:
		result := NewInternalV1OperationsSnapshotsDeleteNoContent()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	case 400:
		result := NewInternalV1OperationsSnapshotsDeleteBadRequest()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result
	case 401:
		result := NewInternalV1OperationsSnapshotsDeleteUnauthorized()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result
	case 403:
		result := NewInternalV1OperationsSnapshotsDeleteForbidden()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result
	case 404:
		result := NewInternalV1OperationsSnapshotsDeleteNotFound()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result
	case 410:
		result := NewInternalV1OperationsSnapshotsDeleteGone()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result
	case 429:
		result := NewInternalV1OperationsSnapshotsDeleteTooManyRequests()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result
	case 500:
		result := NewInternalV1OperationsSnapshotsDeleteInternalServerError()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result
	default:
		return nil, runtime.NewAPIError("[DELETE /internal/v1/operations/snapshots/{resource_crn}] internal.v1.operations.snapshots.delete", response, response.Code())
	}
}

// NewInternalV1OperationsSnapshotsDeleteNoContent creates a InternalV1OperationsSnapshotsDeleteNoContent with default headers values
func NewInternalV1OperationsSnapshotsDeleteNoContent() *InternalV1OperationsSnapshotsDeleteNoContent {
	return &InternalV1OperationsSnapshotsDeleteNoContent{}
}

/*
InternalV1OperationsSnapshotsDeleteNoContent describes a response with status code 204, with default header values.

Deleted
*/
type InternalV1OperationsSnapshotsDeleteNoContent struct {
}

// IsSuccess returns true when this internal v1 operations snapshots delete no content response has a 2xx status code
func (o *InternalV1OperationsSnapshotsDeleteNoContent) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this internal v1 operations snapshots delete no content response has a 3xx status code
func (o *InternalV1OperationsSnapshotsDeleteNoContent) IsRedirect() bool {
	return false
}

// IsClientError returns true when this internal v1 operations snapshots delete no content response has a 4xx status code
func (o *InternalV1OperationsSnapshotsDeleteNoContent) IsClientError() bool {
	return false
}

// IsServerError returns true when this internal v1 operations snapshots delete no content response has a 5xx status code
func (o *InternalV1OperationsSnapshotsDeleteNoContent) IsServerError() bool {
	return false
}

// IsCode returns true when this internal v1 operations snapshots delete no content response a status code equal to that given
func (o *InternalV1OperationsSnapshotsDeleteNoContent) IsCode(code int) bool {
	return code == 204
}

// Code gets the status code for the internal v1 operations snapshots delete no content response
func (o *InternalV1OperationsSnapshotsDeleteNoContent) Code() int {
	return 204
}

func (o *InternalV1OperationsSnapshotsDeleteNoContent) Error() string {
	return fmt.Sprintf("[DELETE /internal/v1/operations/snapshots/{resource_crn}][%d] internalV1OperationsSnapshotsDeleteNoContent", 204)
}

func (o *InternalV1OperationsSnapshotsDeleteNoContent) String() string {
	return fmt.Sprintf("[DELETE /internal/v1/operations/snapshots/{resource_crn}][%d] internalV1OperationsSnapshotsDeleteNoContent", 204)
}

func (o *InternalV1OperationsSnapshotsDeleteNoContent) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewInternalV1OperationsSnapshotsDeleteBadRequest creates a InternalV1OperationsSnapshotsDeleteBadRequest with default headers values
func NewInternalV1OperationsSnapshotsDeleteBadRequest() *InternalV1OperationsSnapshotsDeleteBadRequest {
	return &InternalV1OperationsSnapshotsDeleteBadRequest{}
}

/*
InternalV1OperationsSnapshotsDeleteBadRequest describes a response with status code 400, with default header values.

Bad Request
*/
type InternalV1OperationsSnapshotsDeleteBadRequest struct {
	Payload *models.Error
}

// IsSuccess returns true when this internal v1 operations snapshots delete bad request response has a 2xx status code
func (o *InternalV1OperationsSnapshotsDeleteBadRequest) IsSuccess() bool {
	return false
}

// IsRedirect returns true when this internal v1 operations snapshots delete bad request response has a 3xx status code
func (o *InternalV1OperationsSnapshotsDeleteBadRequest) IsRedirect() bool {
	return false
}

// IsClientError returns true when this internal v1 operations snapshots delete bad request response has a 4xx status code
func (o *InternalV1OperationsSnapshotsDeleteBadRequest) IsClientError() bool {
	return true
}

// IsServerError returns true when this internal v1 operations snapshots delete bad request response has a 5xx status code
func (o *InternalV1OperationsSnapshotsDeleteBadRequest) IsServerError() bool {
	return false
}

// IsCode returns true when this internal v1 operations snapshots delete bad request response a status code equal to that given
func (o *InternalV1OperationsSnapshotsDeleteBadRequest) IsCode(code int) bool {
	return code == 400
}

// Code gets the status code for the internal v1 operations snapshots delete bad request response
func (o *InternalV1OperationsSnapshotsDeleteBadRequest) Code() int {
	return 400
}

func (o *InternalV1OperationsSnapshotsDeleteBadRequest) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /internal/v1/operations/snapshots/{resource_crn}][%d] internalV1OperationsSnapshotsDeleteBadRequest %s", 400, payload)
}

func (o *InternalV1OperationsSnapshotsDeleteBadRequest) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /internal/v1/operations/snapshots/{resource_crn}][%d] internalV1OperationsSnapshotsDeleteBadRequest %s", 400, payload)
}

func (o *InternalV1OperationsSnapshotsDeleteBadRequest) GetPayload() *models.Error {
	return o.Payload
}

func (o *InternalV1OperationsSnapshotsDeleteBadRequest) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Error)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewInternalV1OperationsSnapshotsDeleteUnauthorized creates a InternalV1OperationsSnapshotsDeleteUnauthorized with default headers values
func NewInternalV1OperationsSnapshotsDeleteUnauthorized() *InternalV1OperationsSnapshotsDeleteUnauthorized {
	return &InternalV1OperationsSnapshotsDeleteUnauthorized{}
}

/*
InternalV1OperationsSnapshotsDeleteUnauthorized describes a response with status code 401, with default header values.

Unauthorized
*/
type InternalV1OperationsSnapshotsDeleteUnauthorized struct {
	Payload *models.Error
}

// IsSuccess returns true when this internal v1 operations snapshots delete unauthorized response has a 2xx status code
func (o *InternalV1OperationsSnapshotsDeleteUnauthorized) IsSuccess() bool {
	return false
}

// IsRedirect returns true when this internal v1 operations snapshots delete unauthorized response has a 3xx status code
func (o *InternalV1OperationsSnapshotsDeleteUnauthorized) IsRedirect() bool {
	return false
}

// IsClientError returns true when this internal v1 operations snapshots delete unauthorized response has a 4xx status code
func (o *InternalV1OperationsSnapshotsDeleteUnauthorized) IsClientError() bool {
	return true
}

// IsServerError returns true when this internal v1 operations snapshots delete unauthorized response has a 5xx status code
func (o *InternalV1OperationsSnapshotsDeleteUnauthorized) IsServerError() bool {
	return false
}

// IsCode returns true when this internal v1 operations snapshots delete unauthorized response a status code equal to that given
func (o *InternalV1OperationsSnapshotsDeleteUnauthorized) IsCode(code int) bool {
	return code == 401
}

// Code gets the status code for the internal v1 operations snapshots delete unauthorized response
func (o *InternalV1OperationsSnapshotsDeleteUnauthorized) Code() int {
	return 401
}

func (o *InternalV1OperationsSnapshotsDeleteUnauthorized) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /internal/v1/operations/snapshots/{resource_crn}][%d] internalV1OperationsSnapshotsDeleteUnauthorized %s", 401, payload)
}

func (o *InternalV1OperationsSnapshotsDeleteUnauthorized) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /internal/v1/operations/snapshots/{resource_crn}][%d] internalV1OperationsSnapshotsDeleteUnauthorized %s", 401, payload)
}

func (o *InternalV1OperationsSnapshotsDeleteUnauthorized) GetPayload() *models.Error {
	return o.Payload
}

func (o *InternalV1OperationsSnapshotsDeleteUnauthorized) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Error)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewInternalV1OperationsSnapshotsDeleteForbidden creates a InternalV1OperationsSnapshotsDeleteForbidden with default headers values
func NewInternalV1OperationsSnapshotsDeleteForbidden() *InternalV1OperationsSnapshotsDeleteForbidden {
	return &InternalV1OperationsSnapshotsDeleteForbidden{}
}

/*
InternalV1OperationsSnapshotsDeleteForbidden describes a response with status code 403, with default header values.

Forbidden
*/
type InternalV1OperationsSnapshotsDeleteForbidden struct {
	Payload *models.Error
}

// IsSuccess returns true when this internal v1 operations snapshots delete forbidden response has a 2xx status code
func (o *InternalV1OperationsSnapshotsDeleteForbidden) IsSuccess() bool {
	return false
}

// IsRedirect returns true when this internal v1 operations snapshots delete forbidden response has a 3xx status code
func (o *InternalV1OperationsSnapshotsDeleteForbidden) IsRedirect() bool {
	return false
}

// IsClientError returns true when this internal v1 operations snapshots delete forbidden response has a 4xx status code
func (o *InternalV1OperationsSnapshotsDeleteForbidden) IsClientError() bool {
	return true
}

// IsServerError returns true when this internal v1 operations snapshots delete forbidden response has a 5xx status code
func (o *InternalV1OperationsSnapshotsDeleteForbidden) IsServerError() bool {
	return false
}

// IsCode returns true when this internal v1 operations snapshots delete forbidden response a status code equal to that given
func (o *InternalV1OperationsSnapshotsDeleteForbidden) IsCode(code int) bool {
	return code == 403
}

// Code gets the status code for the internal v1 operations snapshots delete forbidden response
func (o *InternalV1OperationsSnapshotsDeleteForbidden) Code() int {
	return 403
}

func (o *InternalV1OperationsSnapshotsDeleteForbidden) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /internal/v1/operations/snapshots/{resource_crn}][%d] internalV1OperationsSnapshotsDeleteForbidden %s", 403, payload)
}

func (o *InternalV1OperationsSnapshotsDeleteForbidden) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /internal/v1/operations/snapshots/{resource_crn}][%d] internalV1OperationsSnapshotsDeleteForbidden %s", 403, payload)
}

func (o *InternalV1OperationsSnapshotsDeleteForbidden) GetPayload() *models.Error {
	return o.Payload
}

func (o *InternalV1OperationsSnapshotsDeleteForbidden) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Error)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewInternalV1OperationsSnapshotsDeleteNotFound creates a InternalV1OperationsSnapshotsDeleteNotFound with default headers values
func NewInternalV1OperationsSnapshotsDeleteNotFound() *InternalV1OperationsSnapshotsDeleteNotFound {
	return &InternalV1OperationsSnapshotsDeleteNotFound{}
}

/*
InternalV1OperationsSnapshotsDeleteNotFound describes a response with status code 404, with default header values.

Not Found
*/
type InternalV1OperationsSnapshotsDeleteNotFound struct {
	Payload *models.Error
}

// IsSuccess returns true when this internal v1 operations snapshots delete not found response has a 2xx status code
func (o *InternalV1OperationsSnapshotsDeleteNotFound) IsSuccess() bool {
	return false
}

// IsRedirect returns true when this internal v1 operations snapshots delete not found response has a 3xx status code
func (o *InternalV1OperationsSnapshotsDeleteNotFound) IsRedirect() bool {
	return false
}

// IsClientError returns true when this internal v1 operations snapshots delete not found response has a 4xx status code
func (o *InternalV1OperationsSnapshotsDeleteNotFound) IsClientError() bool {
	return true
}

// IsServerError returns true when this internal v1 operations snapshots delete not found response has a 5xx status code
func (o *InternalV1OperationsSnapshotsDeleteNotFound) IsServerError() bool {
	return false
}

// IsCode returns true when this internal v1 operations snapshots delete not found response a status code equal to that given
func (o *InternalV1OperationsSnapshotsDeleteNotFound) IsCode(code int) bool {
	return code == 404
}

// Code gets the status code for the internal v1 operations snapshots delete not found response
func (o *InternalV1OperationsSnapshotsDeleteNotFound) Code() int {
	return 404
}

func (o *InternalV1OperationsSnapshotsDeleteNotFound) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /internal/v1/operations/snapshots/{resource_crn}][%d] internalV1OperationsSnapshotsDeleteNotFound %s", 404, payload)
}

func (o *InternalV1OperationsSnapshotsDeleteNotFound) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /internal/v1/operations/snapshots/{resource_crn}][%d] internalV1OperationsSnapshotsDeleteNotFound %s", 404, payload)
}

func (o *InternalV1OperationsSnapshotsDeleteNotFound) GetPayload() *models.Error {
	return o.Payload
}

func (o *InternalV1OperationsSnapshotsDeleteNotFound) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Error)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewInternalV1OperationsSnapshotsDeleteGone creates a InternalV1OperationsSnapshotsDeleteGone with default headers values
func NewInternalV1OperationsSnapshotsDeleteGone() *InternalV1OperationsSnapshotsDeleteGone {
	return &InternalV1OperationsSnapshotsDeleteGone{}
}

/*
InternalV1OperationsSnapshotsDeleteGone describes a response with status code 410, with default header values.

Gone
*/
type InternalV1OperationsSnapshotsDeleteGone struct {
	Payload *models.Error
}

// IsSuccess returns true when this internal v1 operations snapshots delete gone response has a 2xx status code
func (o *InternalV1OperationsSnapshotsDeleteGone) IsSuccess() bool {
	return false
}

// IsRedirect returns true when this internal v1 operations snapshots delete gone response has a 3xx status code
func (o *InternalV1OperationsSnapshotsDeleteGone) IsRedirect() bool {
	return false
}

// IsClientError returns true when this internal v1 operations snapshots delete gone response has a 4xx status code
func (o *InternalV1OperationsSnapshotsDeleteGone) IsClientError() bool {
	return true
}

// IsServerError returns true when this internal v1 operations snapshots delete gone response has a 5xx status code
func (o *InternalV1OperationsSnapshotsDeleteGone) IsServerError() bool {
	return false
}

// IsCode returns true when this internal v1 operations snapshots delete gone response a status code equal to that given
func (o *InternalV1OperationsSnapshotsDeleteGone) IsCode(code int) bool {
	return code == 410
}

// Code gets the status code for the internal v1 operations snapshots delete gone response
func (o *InternalV1OperationsSnapshotsDeleteGone) Code() int {
	return 410
}

func (o *InternalV1OperationsSnapshotsDeleteGone) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /internal/v1/operations/snapshots/{resource_crn}][%d] internalV1OperationsSnapshotsDeleteGone %s", 410, payload)
}

func (o *InternalV1OperationsSnapshotsDeleteGone) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /internal/v1/operations/snapshots/{resource_crn}][%d] internalV1OperationsSnapshotsDeleteGone %s", 410, payload)
}

func (o *InternalV1OperationsSnapshotsDeleteGone) GetPayload() *models.Error {
	return o.Payload
}

func (o *InternalV1OperationsSnapshotsDeleteGone) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Error)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewInternalV1OperationsSnapshotsDeleteTooManyRequests creates a InternalV1OperationsSnapshotsDeleteTooManyRequests with default headers values
func NewInternalV1OperationsSnapshotsDeleteTooManyRequests() *InternalV1OperationsSnapshotsDeleteTooManyRequests {
	return &InternalV1OperationsSnapshotsDeleteTooManyRequests{}
}

/*
InternalV1OperationsSnapshotsDeleteTooManyRequests describes a response with status code 429, with default header values.

Too Many Requests
*/
type InternalV1OperationsSnapshotsDeleteTooManyRequests struct {
	Payload *models.Error
}

// IsSuccess returns true when this internal v1 operations snapshots delete too many requests response has a 2xx status code
func (o *InternalV1OperationsSnapshotsDeleteTooManyRequests) IsSuccess() bool {
	return false
}

// IsRedirect returns true when this internal v1 operations snapshots delete too many requests response has a 3xx status code
func (o *InternalV1OperationsSnapshotsDeleteTooManyRequests) IsRedirect() bool {
	return false
}

// IsClientError returns true when this internal v1 operations snapshots delete too many requests response has a 4xx status code
func (o *InternalV1OperationsSnapshotsDeleteTooManyRequests) IsClientError() bool {
	return true
}

// IsServerError returns true when this internal v1 operations snapshots delete too many requests response has a 5xx status code
func (o *InternalV1OperationsSnapshotsDeleteTooManyRequests) IsServerError() bool {
	return false
}

// IsCode returns true when this internal v1 operations snapshots delete too many requests response a status code equal to that given
func (o *InternalV1OperationsSnapshotsDeleteTooManyRequests) IsCode(code int) bool {
	return code == 429
}

// Code gets the status code for the internal v1 operations snapshots delete too many requests response
func (o *InternalV1OperationsSnapshotsDeleteTooManyRequests) Code() int {
	return 429
}

func (o *InternalV1OperationsSnapshotsDeleteTooManyRequests) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /internal/v1/operations/snapshots/{resource_crn}][%d] internalV1OperationsSnapshotsDeleteTooManyRequests %s", 429, payload)
}

func (o *InternalV1OperationsSnapshotsDeleteTooManyRequests) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /internal/v1/operations/snapshots/{resource_crn}][%d] internalV1OperationsSnapshotsDeleteTooManyRequests %s", 429, payload)
}

func (o *InternalV1OperationsSnapshotsDeleteTooManyRequests) GetPayload() *models.Error {
	return o.Payload
}

func (o *InternalV1OperationsSnapshotsDeleteTooManyRequests) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Error)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewInternalV1OperationsSnapshotsDeleteInternalServerError creates a InternalV1OperationsSnapshotsDeleteInternalServerError with default headers values
func NewInternalV1OperationsSnapshotsDeleteInternalServerError() *InternalV1OperationsSnapshotsDeleteInternalServerError {
	return &InternalV1OperationsSnapshotsDeleteInternalServerError{}
}

/*
InternalV1OperationsSnapshotsDeleteInternalServerError describes a response with status code 500, with default header values.

Internal Server Error
*/
type InternalV1OperationsSnapshotsDeleteInternalServerError struct {
	Payload *models.Error
}

// IsSuccess returns true when this internal v1 operations snapshots delete internal server error response has a 2xx status code
func (o *InternalV1OperationsSnapshotsDeleteInternalServerError) IsSuccess() bool {
	return false
}

// IsRedirect returns true when this internal v1 operations snapshots delete internal server error response has a 3xx status code
func (o *InternalV1OperationsSnapshotsDeleteInternalServerError) IsRedirect() bool {
	return false
}

// IsClientError returns true when this internal v1 operations snapshots delete internal server error response has a 4xx status code
func (o *InternalV1OperationsSnapshotsDeleteInternalServerError) IsClientError() bool {
	return false
}

// IsServerError returns true when this internal v1 operations snapshots delete internal server error response has a 5xx status code
func (o *InternalV1OperationsSnapshotsDeleteInternalServerError) IsServerError() bool {
	return true
}

// IsCode returns true when this internal v1 operations snapshots delete internal server error response a status code equal to that given
func (o *InternalV1OperationsSnapshotsDeleteInternalServerError) IsCode(code int) bool {
	return code == 500
}

// Code gets the status code for the internal v1 operations snapshots delete internal server error response
func (o *InternalV1OperationsSnapshotsDeleteInternalServerError) Code() int {
	return 500
}

func (o *InternalV1OperationsSnapshotsDeleteInternalServerError) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /internal/v1/operations/snapshots/{resource_crn}][%d] internalV1OperationsSnapshotsDeleteInternalServerError %s", 500, payload)
}

func (o *InternalV1OperationsSnapshotsDeleteInternalServerError) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[DELETE /internal/v1/operations/snapshots/{resource_crn}][%d] internalV1OperationsSnapshotsDeleteInternalServerError %s", 500, payload)
}

func (o *InternalV1OperationsSnapshotsDeleteInternalServerError) GetPayload() *models.Error {
	return o.Payload
}

func (o *InternalV1OperationsSnapshotsDeleteInternalServerError) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Error)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
