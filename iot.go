// Copyright 2023 ClearBlade Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// Copyright 2023 Google LLC.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package iot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/clearblade/go-iot/cblib/gensupport"
	"github.com/clearblade/go-iot/cblib/googleapi"
	"github.com/clearblade/go-iot/cblib/path_template"
)

type ServiceAccountCredentials struct {
	SystemKey string `json:"systemKey"`
	Token     string `json:"token"`
	Url       string `json:"url"`
	Project   string `json:"project"`
}

type RegistryUserCredentials struct {
	SystemKey string `json:"systemKey"`
	Token     string `json:"serviceAccountToken"`
	Url       string `json:"url"`
}

func loadCredentialsJSON() (*ServiceAccountCredentials, error) {
	envVal := os.Getenv("CLEARBLADE_API_CREDENTIALS_JSON")
	if envVal == "" {
		return nil, errors.New("must supply service account credentials via CLEARBLADE_API_CREDENTIALS_JSON environment variable")
	}

	var credentials ServiceAccountCredentials
	err := json.Unmarshal([]byte(envVal), &credentials)
	if err != nil {
		return nil, fmt.Errorf("content of CLEARBLADE_API_CREDENTIALS_JSON is invalid. Please make sure it is a json object with the properties systemKey, token, url, and project: %v", err)
	}
	return &credentials, nil
}

func loadServiceAccountCredentials() (*ServiceAccountCredentials, error) {
	configFilePath := os.Getenv("CLEARBLADE_CONFIGURATION")
	var err error
	configFile, err := os.Open(configFilePath)
	if err != nil {
		return nil, errors.New("must supply service account credentials via constructor or CLEARBLADE_CONFIGURATION environment variable")
	}
	defer configFile.Close()

	byteValue, _ := io.ReadAll(configFile)
	var credentials ServiceAccountCredentials
	err = json.Unmarshal(byteValue, &credentials)
	if err != nil {
		return nil, errors.New("File loaded from " + configFilePath + " is invalid. Please make sure it is a json file with the properties systemKey, token, url, and project")
	}
	return &credentials, nil
}

func createHTTPError(res *http.Response) error {
	bytes, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	var body map[string]struct {
		Code    int64
		Message string
		Status  string
	}
	err = json.Unmarshal(bytes, &body)
	if err != nil {
		return err
	}
	return errors.New(fmt.Sprintf("clearbladeiot: Error %d: %s, %s\n", body["error"].Code, body["error"].Message, body["error"].Status))

}

func GetRegistryCredentials(registry string, region string, s *Service) *RegistryUserCredentials {
	cacheKey := fmt.Sprintf("%s-%s", region, registry)
	if s.RegistryUserCache[cacheKey] != nil {
		return s.RegistryUserCache[cacheKey]
	}
	requestBody, _ := json.Marshal(map[string]string{
		"region": region, "registry": registry, "project": s.ServiceAccountCredentials.Project,
	})
	url := fmt.Sprintf("%s/api/v/1/code/%s/getRegistryCredentials", s.ServiceAccountCredentials.Url, s.ServiceAccountCredentials.SystemKey)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(requestBody))
	req.Header.Add("ClearBlade-UserToken", s.ServiceAccountCredentials.Token)
	resp, err := s.client.Do(req)
	if err != nil {
		log.Fatalln(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}
	var credentials RegistryUserCredentials
	_ = json.Unmarshal(body, &credentials)

	s.RegistryUserCache[cacheKey] = &credentials

	return &credentials
}

// NewServiceWithJSONCredentials creates a new Service with JSON credentials
func NewServiceWithJSONCredentials(ctx context.Context) (*Service, error) {
	credentials, err := loadCredentialsJSON()
	if err != nil {
		return nil, err
	}
	return newservice(credentials)

}

// NewServiceWithServiceAccountFileCredentials creates a new Service with service file account credentials
func NewServiceWithServiceAccountFileCredentials(ctx context.Context) (*Service, error) {
	credentials, err := loadServiceAccountCredentials()
	if err != nil {
		return nil, err
	}
	return newservice(credentials)
}

func newservice(credentials *ServiceAccountCredentials) (*Service, error) {
	s := &Service{
		client:                    http.DefaultClient,
		RegistryUserCache:         make(map[string]*RegistryUserCredentials),
		ServiceAccountCredentials: credentials,
	}

	devicePathTemplate, err := path_template.NewPathTemplate("projects/{project}/locations/{location}/registries/{registry}/devices/{device}")
	if err != nil {
		return nil, err
	}
	locationPathTemplate, err := path_template.NewPathTemplate("projects/{project}/locations/{location}")
	if err != nil {
		return nil, err
	}
	registryPathTemplate, err := path_template.NewPathTemplate("projects/{project}/locations/{location}/registries/{registry}")
	if err != nil {
		return nil, err
	}
	s.TemplatePaths.DevicePathTemplate = devicePathTemplate
	s.TemplatePaths.LocationPathTemplate = locationPathTemplate
	s.TemplatePaths.RegistryPathTemplate = registryPathTemplate
	s.Projects = NewProjectsService(s)
	return s, nil
}

type Service struct {
	client                    *http.Client
	RegistryUserCache         map[string]*RegistryUserCredentials
	ServiceAccountCredentials *ServiceAccountCredentials
	TemplatePaths             struct {
		DevicePathTemplate   *path_template.PathTemplate
		LocationPathTemplate *path_template.PathTemplate
		RegistryPathTemplate *path_template.PathTemplate
	}

	Projects *ProjectsService
}

func NewProjectsService(s *Service) *ProjectsService {
	rs := &ProjectsService{s: s}
	rs.Locations = NewProjectsLocationsService(s)
	return rs
}

type ProjectsService struct {
	s *Service

	Locations *ProjectsLocationsService
}

func NewProjectsLocationsService(s *Service) *ProjectsLocationsService {
	rs := &ProjectsLocationsService{s: s}
	rs.Registries = NewProjectsLocationsRegistriesService(s)
	return rs
}

type ProjectsLocationsService struct {
	s *Service

	Registries *ProjectsLocationsRegistriesService
}

func NewProjectsLocationsRegistriesService(s *Service) *ProjectsLocationsRegistriesService {
	rs := &ProjectsLocationsRegistriesService{s: s}
	rs.Devices = NewProjectsLocationsRegistriesDevicesService(s)
	rs.Groups = NewProjectsLocationsRegistriesGroupsService(s)
	return rs
}

type ProjectsLocationsRegistriesService struct {
	s *Service

	Devices *ProjectsLocationsRegistriesDevicesService

	Groups *ProjectsLocationsRegistriesGroupsService
}

func NewProjectsLocationsRegistriesDevicesService(s *Service) *ProjectsLocationsRegistriesDevicesService {
	rs := &ProjectsLocationsRegistriesDevicesService{s: s}
	rs.ConfigVersions = NewProjectsLocationsRegistriesDevicesConfigVersionsService(s)
	rs.States = NewProjectsLocationsRegistriesDevicesStatesService(s)
	return rs
}

type ProjectsLocationsRegistriesDevicesService struct {
	s *Service

	ConfigVersions *ProjectsLocationsRegistriesDevicesConfigVersionsService

	States *ProjectsLocationsRegistriesDevicesStatesService
}

func NewProjectsLocationsRegistriesDevicesConfigVersionsService(s *Service) *ProjectsLocationsRegistriesDevicesConfigVersionsService {
	rs := &ProjectsLocationsRegistriesDevicesConfigVersionsService{s: s}
	return rs
}

type ProjectsLocationsRegistriesDevicesConfigVersionsService struct {
	s *Service
}

func NewProjectsLocationsRegistriesDevicesStatesService(s *Service) *ProjectsLocationsRegistriesDevicesStatesService {
	rs := &ProjectsLocationsRegistriesDevicesStatesService{s: s}
	return rs
}

type ProjectsLocationsRegistriesDevicesStatesService struct {
	s *Service
}

func NewProjectsLocationsRegistriesGroupsService(s *Service) *ProjectsLocationsRegistriesGroupsService {
	rs := &ProjectsLocationsRegistriesGroupsService{s: s}
	rs.Devices = NewProjectsLocationsRegistriesGroupsDevicesService(s)
	return rs
}

type ProjectsLocationsRegistriesGroupsService struct {
	s *Service

	Devices *ProjectsLocationsRegistriesGroupsDevicesService
}

func NewProjectsLocationsRegistriesGroupsDevicesService(s *Service) *ProjectsLocationsRegistriesGroupsDevicesService {
	rs := &ProjectsLocationsRegistriesGroupsDevicesService{s: s}
	return rs
}

type ProjectsLocationsRegistriesGroupsDevicesService struct {
	s *Service
}

// BindDeviceToGatewayRequest: Request for `BindDeviceToGateway`.
type BindDeviceToGatewayRequest struct {
	// DeviceId: Required. The device to associate with the specified
	// gateway. The value of `device_id` can be either the device numeric ID
	// or the user-defined device identifier.
	DeviceId string `json:"deviceId,omitempty"`

	// GatewayId: Required. The value of `gateway_id` can be either the
	// device numeric ID or the user-defined device identifier.
	GatewayId string `json:"gatewayId,omitempty"`

	// ForceSendFields is a list of field names (e.g. "DeviceId") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "DeviceId") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *BindDeviceToGatewayRequest) MarshalJSON() ([]byte, error) {
	type NoMethod BindDeviceToGatewayRequest
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// BindDeviceToGatewayResponse: Response for `BindDeviceToGateway`.
type BindDeviceToGatewayResponse struct {
	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`
}

// Binding: Associates `members`, or principals, with a `role`.
type Binding struct {
	// Condition: The condition that is associated with this binding. If the
	// condition evaluates to `true`, then this binding applies to the
	// current request. If the condition evaluates to `false`, then this
	// binding does not apply to the current request. However, a different
	// role binding might grant the same role to one or more of the
	// principals in this binding. To learn which resources support
	// conditions in their IAM policies, see the IAM documentation
	// (https://cloud.google.com/iam/help/conditions/resource-policies).
	Condition *Expr `json:"condition,omitempty"`

	// Members: Specifies the principals requesting access for a Google
	// Cloud resource. `members` can have the following values: *
	// `allUsers`: A special identifier that represents anyone who is on the
	// internet; with or without a Google account. *
	// `allAuthenticatedUsers`: A special identifier that represents anyone
	// who is authenticated with a Google account or a service account. Does
	// not include identities that come from external identity providers
	// (IdPs) through identity federation. * `user:{emailid}`: An email
	// address that represents a specific Google account. For example,
	// `alice@example.com` . * `serviceAccount:{emailid}`: An email address
	// that represents a Google service account. For example,
	// `my-other-app@appspot.gserviceaccount.com`. *
	// `serviceAccount:{projectid}.svc.id.goog[{namespace}/{kubernetes-sa}]`:
	//  An identifier for a Kubernetes service account
	// (https://cloud.google.com/kubernetes-engine/docs/how-to/kubernetes-service-accounts).
	// For example, `my-project.svc.id.goog[my-namespace/my-kubernetes-sa]`.
	// * `group:{emailid}`: An email address that represents a Google group.
	// For example, `admins@example.com`. *
	// `deleted:user:{emailid}?uid={uniqueid}`: An email address (plus
	// unique identifier) representing a user that has been recently
	// deleted. For example, `alice@example.com?uid=123456789012345678901`.
	// If the user is recovered, this value reverts to `user:{emailid}` and
	// the recovered user retains the role in the binding. *
	// `deleted:serviceAccount:{emailid}?uid={uniqueid}`: An email address
	// (plus unique identifier) representing a service account that has been
	// recently deleted. For example,
	// `my-other-app@appspot.gserviceaccount.com?uid=123456789012345678901`.
	// If the service account is undeleted, this value reverts to
	// `serviceAccount:{emailid}` and the undeleted service account retains
	// the role in the binding. * `deleted:group:{emailid}?uid={uniqueid}`:
	// An email address (plus unique identifier) representing a Google group
	// that has been recently deleted. For example,
	// `admins@example.com?uid=123456789012345678901`. If the group is
	// recovered, this value reverts to `group:{emailid}` and the recovered
	// group retains the role in the binding. * `domain:{domain}`: The G
	// Suite domain (primary) that represents all the users of that domain.
	// For example, `google.com` or `example.com`.
	Members []string `json:"members,omitempty"`

	// Role: Role that is assigned to the list of `members`, or principals.
	// For example, `roles/viewer`, `roles/editor`, or `roles/owner`.
	Role string `json:"role,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Condition") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Condition") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *Binding) MarshalJSON() ([]byte, error) {
	type NoMethod Binding
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// Device: The device resource.
type Device struct {
	// Blocked: If a device is blocked, connections or requests from this
	// device will fail. Can be used to temporarily prevent the device from
	// connecting if, for example, the sensor is generating bad data and
	// needs maintenance.
	Blocked bool `json:"blocked"`

	// Config: The most recent device configuration, which is eventually
	// sent from Cloud IoT Core to the device. If not present on creation,
	// the configuration will be initialized with an empty payload and
	// version value of `1`. To update this field after creation, use the
	// `DeviceManager.ModifyCloudToDeviceConfig` method.
	Config *DeviceConfig `json:"config,omitempty"`

	// Credentials: The credentials used to authenticate this device. To
	// allow credential rotation without interruption, multiple device
	// credentials can be bound to this device. No more than 3 credentials
	// can be bound to a single device at a time. When new credentials are
	// added to a device, they are verified against the registry
	// credentials. For details, see the description of the
	// `DeviceRegistry.credentials` field.
	Credentials []*DeviceCredential `json:"credentials,omitempty"`

	// GatewayConfig: Gateway-related configuration and state.
	GatewayConfig *GatewayConfig `json:"gatewayConfig,omitempty"`

	// Id: The user-defined device identifier. The device ID must be unique
	// within a device registry.
	Id string `json:"id,omitempty"`

	// LastConfigAckTime: [Output only] The last time a cloud-to-device
	// config version acknowledgment was received from the device. This
	// field is only for configurations sent through MQTT.
	LastConfigAckTime string `json:"lastConfigAckTime,omitempty"`

	// LastConfigSendTime: [Output only] The last time a cloud-to-device
	// config version was sent to the device.
	LastConfigSendTime string `json:"lastConfigSendTime,omitempty"`

	// LastErrorStatus: [Output only] The error message of the most recent
	// error, such as a failure to publish to Cloud Pub/Sub.
	// 'last_error_time' is the timestamp of this field. If no errors have
	// occurred, this field has an empty message and the status code 0 ==
	// OK. Otherwise, this field is expected to have a status code other
	// than OK.
	LastErrorStatus *Status `json:"lastErrorStatus,omitempty"`

	// LastErrorTime: [Output only] The time the most recent error occurred,
	// such as a failure to publish to Cloud Pub/Sub. This field is the
	// timestamp of 'last_error_status'.
	LastErrorTime string `json:"lastErrorTime,omitempty"`

	// LastEventTime: [Output only] The last time a telemetry event was
	// received. Timestamps are periodically collected and written to
	// storage; they may be stale by a few minutes.
	LastEventTime string `json:"lastEventTime,omitempty"`

	// LastHeartbeatTime: [Output only] The last time an MQTT `PINGREQ` was
	// received. This field applies only to devices connecting through MQTT.
	// MQTT clients usually only send `PINGREQ` messages if the connection
	// is idle, and no other messages have been sent. Timestamps are
	// periodically collected and written to storage; they may be stale by a
	// few minutes.
	LastHeartbeatTime string `json:"lastHeartbeatTime,omitempty"`

	// LastStateTime: [Output only] The last time a state event was
	// received. Timestamps are periodically collected and written to
	// storage; they may be stale by a few minutes.
	LastStateTime string `json:"lastStateTime,omitempty"`

	// LogLevel: **Beta Feature** The logging verbosity for device activity.
	// If unspecified, DeviceRegistry.log_level will be used.
	//
	// Possible values:
	//   "LOG_LEVEL_UNSPECIFIED" - No logging specified. If not specified,
	// logging will be disabled.
	//   "NONE" - Disables logging.
	//   "ERROR" - Error events will be logged.
	//   "INFO" - Informational events will be logged, such as connections
	// and disconnections.
	//   "DEBUG" - All events will be logged.
	LogLevel string `json:"logLevel,omitempty"`

	// Metadata: The metadata key-value pairs assigned to the device. This
	// metadata is not interpreted or indexed by Cloud IoT Core. It can be
	// used to add contextual information for the device. Keys must conform
	// to the regular expression a-zA-Z+ and be less than 128 bytes in
	// length. Values are free-form strings. Each value must be less than or
	// equal to 32 KB in size. The total size of all keys and values must be
	// less than 256 KB, and the maximum number of key-value pairs is 500.
	Metadata map[string]string `json:"metadata,omitempty"`

	// Name: The resource path name. For example,
	// `projects/p1/locations/us-central1/registries/registry0/devices/dev0`
	// or
	// `projects/p1/locations/us-central1/registries/registry0/devices/{num_i
	// d}`. When `name` is populated as a response from the service, it
	// always ends in the device numeric ID.
	Name string `json:"name,omitempty"`

	// NumId: [Output only] A server-defined unique numeric ID for the
	// device. This is a more compact way to identify devices, and it is
	// globally unique.
	NumId uint64 `json:"numId,omitempty,string"`

	// State: [Output only] The state most recently received from the
	// device. If no state has been reported, this field is not present.
	State *DeviceState `json:"state,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Blocked") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Blocked") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *Device) MarshalJSON() ([]byte, error) {
	type NoMethod Device
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// DeviceConfig: The device configuration. Eventually delivered to
// devices.
type DeviceConfig struct {
	// BinaryData: The device configuration data.
	BinaryData string `json:"binaryData,omitempty"`

	// CloudUpdateTime: [Output only] The time at which this configuration
	// version was updated in Cloud IoT Core. This timestamp is set by the
	// server.
	CloudUpdateTime string `json:"cloudUpdateTime,omitempty"`

	// DeviceAckTime: [Output only] The time at which Cloud IoT Core
	// received the acknowledgment from the device, indicating that the
	// device has received this configuration version. If this field is not
	// present, the device has not yet acknowledged that it received this
	// version. Note that when the config was sent to the device, many
	// config versions may have been available in Cloud IoT Core while the
	// device was disconnected, and on connection, only the latest version
	// is sent to the device. Some versions may never be sent to the device,
	// and therefore are never acknowledged. This timestamp is set by Cloud
	// IoT Core.
	DeviceAckTime string `json:"deviceAckTime,omitempty"`

	// Version: [Output only] The version of this update. The version number
	// is assigned by the server, and is always greater than 0 after device
	// creation. The version must be 0 on the `CreateDevice` request if a
	// `config` is specified; the response of `CreateDevice` will always
	// have a value of 1.
	Version int64 `json:"version,omitempty,string"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "BinaryData") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "BinaryData") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *DeviceConfig) MarshalJSON() ([]byte, error) {
	type NoMethod DeviceConfig
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// DeviceCredential: A server-stored device credential used for
// authentication.
type DeviceCredential struct {
	// ExpirationTime: [Optional] The time at which this credential becomes
	// invalid. This credential will be ignored for new client
	// authentication requests after this timestamp; however, it will not be
	// automatically deleted.
	ExpirationTime string `json:"expirationTime,omitempty"`

	// PublicKey: A public key used to verify the signature of JSON Web
	// Tokens (JWTs). When adding a new device credential, either via device
	// creation or via modifications, this public key credential may be
	// required to be signed by one of the registry level certificates. More
	// specifically, if the registry contains at least one certificate, any
	// new device credential must be signed by one of the registry
	// certificates. As a result, when the registry contains certificates,
	// only X.509 certificates are accepted as device credentials. However,
	// if the registry does not contain a certificate, self-signed
	// certificates and public keys will be accepted. New device credentials
	// must be different from every registry-level certificate.
	PublicKey *PublicKeyCredential `json:"publicKey,omitempty"`

	// ForceSendFields is a list of field names (e.g. "ExpirationTime") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "ExpirationTime") to
	// include in API requests with the JSON null value. By default, fields
	// with empty values are omitted from API requests. However, any field
	// with an empty value appearing in NullFields will be sent to the
	// server as null. It is an error if a field in this list has a
	// non-empty value. This may be used to include null fields in Patch
	// requests.
	NullFields []string `json:"-"`
}

func (s *DeviceCredential) MarshalJSON() ([]byte, error) {
	type NoMethod DeviceCredential
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// DeviceRegistry: A container for a group of devices.
type DeviceRegistry struct {
	// Credentials: The credentials used to verify the device credentials.
	// No more than 10 credentials can be bound to a single registry at a
	// time. The verification process occurs at the time of device creation
	// or update. If this field is empty, no verification is performed.
	// Otherwise, the credentials of a newly created device or added
	// credentials of an updated device should be signed with one of these
	// registry credentials. Note, however, that existing devices will never
	// be affected by modifications to this list of credentials: after a
	// device has been successfully created in a registry, it should be able
	// to connect even if its registry credentials are revoked, deleted, or
	// modified.
	Credentials []*RegistryCredential `json:"credentials,omitempty"`

	// EventNotificationConfigs: The configuration for notification of
	// telemetry events received from the device. All telemetry events that
	// were successfully published by the device and acknowledged by Cloud
	// IoT Core are guaranteed to be delivered to Cloud Pub/Sub. If multiple
	// configurations match a message, only the first matching configuration
	// is used. If you try to publish a device telemetry event using MQTT
	// without specifying a Cloud Pub/Sub topic for the device's registry,
	// the connection closes automatically. If you try to do so using an
	// HTTP connection, an error is returned. Up to 10 configurations may be
	// provided.
	EventNotificationConfigs []*EventNotificationConfig `json:"eventNotificationConfigs,omitempty"`

	// HttpConfig: The DeviceService (HTTP) configuration for this device
	// registry.
	HttpConfig *HttpConfig `json:"httpConfig,omitempty"`

	// Id: The identifier of this device registry. For example,
	// `myRegistry`.
	Id string `json:"id,omitempty"`

	// LogLevel: **Beta Feature** The default logging verbosity for activity
	// from devices in this registry. The verbosity level can be overridden
	// by Device.log_level.
	//
	// Possible values:
	//   "LOG_LEVEL_UNSPECIFIED" - No logging specified. If not specified,
	// logging will be disabled.
	//   "NONE" - Disables logging.
	//   "ERROR" - Error events will be logged.
	//   "INFO" - Informational events will be logged, such as connections
	// and disconnections.
	//   "DEBUG" - All events will be logged.
	LogLevel string `json:"logLevel,omitempty"`

	// MqttConfig: The MQTT configuration for this device registry.
	MqttConfig *MqttConfig `json:"mqttConfig,omitempty"`

	// Name: The resource path name. For example,
	// `projects/example-project/locations/us-central1/registries/my-registry
	// `.
	Name string `json:"name,omitempty"`

	// StateNotificationConfig: The configuration for notification of new
	// states received from the device. State updates are guaranteed to be
	// stored in the state history, but notifications to Cloud Pub/Sub are
	// not guaranteed. For example, if permissions are misconfigured or the
	// specified topic doesn't exist, no notification will be published but
	// the state will still be stored in Cloud IoT Core.
	StateNotificationConfig *StateNotificationConfig `json:"stateNotificationConfig,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Credentials") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Credentials") to include
	// in API requests with the JSON null value. By default, fields with
	// empty values are omitted from API requests. However, any field with
	// an empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *DeviceRegistry) MarshalJSON() ([]byte, error) {
	type NoMethod DeviceRegistry
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// DeviceState: The device state, as reported by the device.
type DeviceState struct {
	// BinaryData: The device state data.
	BinaryData string `json:"binaryData,omitempty"`

	// UpdateTime: [Output only] The time at which this state version was
	// updated in Cloud IoT Core.
	UpdateTime string `json:"updateTime,omitempty"`

	// ForceSendFields is a list of field names (e.g. "BinaryData") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "BinaryData") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *DeviceState) MarshalJSON() ([]byte, error) {
	type NoMethod DeviceState
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// Empty: A generic empty message that you can re-use to avoid defining
// duplicated empty messages in your APIs. A typical example is to use
// it as the request or the response type of an API method. For
// instance: service Foo { rpc Bar(google.protobuf.Empty) returns
// (google.protobuf.Empty); }
type Empty struct {
	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`
}

// EventNotificationConfig: The configuration for forwarding telemetry
// events.
type EventNotificationConfig struct {
	// PubsubTopicName: A Cloud Pub/Sub topic name. For example,
	// `projects/myProject/topics/deviceEvents`.
	PubsubTopicName string `json:"pubsubTopicName,omitempty"`

	// SubfolderMatches: If the subfolder name matches this string exactly,
	// this configuration will be used. The string must not include the
	// leading '/' character. If empty, all strings are matched. This field
	// is used only for telemetry events; subfolders are not supported for
	// state changes.
	SubfolderMatches string `json:"subfolderMatches,omitempty"`

	// ForceSendFields is a list of field names (e.g. "PubsubTopicName") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "PubsubTopicName") to
	// include in API requests with the JSON null value. By default, fields
	// with empty values are omitted from API requests. However, any field
	// with an empty value appearing in NullFields will be sent to the
	// server as null. It is an error if a field in this list has a
	// non-empty value. This may be used to include null fields in Patch
	// requests.
	NullFields []string `json:"-"`
}

func (s *EventNotificationConfig) MarshalJSON() ([]byte, error) {
	type NoMethod EventNotificationConfig
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// Expr: Represents a textual expression in the Common Expression
// Language (CEL) syntax. CEL is a C-like expression language. The
// syntax and semantics of CEL are documented at
// https://github.com/google/cel-spec. Example (Comparison): title:
// "Summary size limit" description: "Determines if a summary is less
// than 100 chars" expression: "document.summary.size() < 100" Example
// (Equality): title: "Requestor is owner" description: "Determines if
// requestor is the document owner" expression: "document.owner ==
// request.auth.claims.email" Example (Logic): title: "Public documents"
// description: "Determine whether the document should be publicly
// visible" expression: "document.type != 'private' && document.type !=
// 'internal'" Example (Data Manipulation): title: "Notification string"
// description: "Create a notification string with a timestamp."
// expression: "'New message received at ' +
// string(document.create_time)" The exact variables and functions that
// may be referenced within an expression are determined by the service
// that evaluates it. See the service documentation for additional
// information.
type Expr struct {
	// Description: Optional. Description of the expression. This is a
	// longer text which describes the expression, e.g. when hovered over it
	// in a UI.
	Description string `json:"description,omitempty"`

	// Expression: Textual representation of an expression in Common
	// Expression Language syntax.
	Expression string `json:"expression,omitempty"`

	// Location: Optional. String indicating the location of the expression
	// for error reporting, e.g. a file name and a position in the file.
	Location string `json:"location,omitempty"`

	// Title: Optional. Title for the expression, i.e. a short string
	// describing its purpose. This can be used e.g. in UIs which allow to
	// enter the expression.
	Title string `json:"title,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Description") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Description") to include
	// in API requests with the JSON null value. By default, fields with
	// empty values are omitted from API requests. However, any field with
	// an empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *Expr) MarshalJSON() ([]byte, error) {
	type NoMethod Expr
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// GatewayConfig: Gateway-related configuration and state.
type GatewayConfig struct {
	// GatewayAuthMethod: Indicates how to authorize and/or authenticate
	// devices to access the gateway.
	//
	// Possible values:
	//   "GATEWAY_AUTH_METHOD_UNSPECIFIED" - No authentication/authorization
	// method specified. No devices are allowed to access the gateway.
	//   "ASSOCIATION_ONLY" - The device is authenticated through the
	// gateway association only. Device credentials are ignored even if
	// provided.
	//   "DEVICE_AUTH_TOKEN_ONLY" - The device is authenticated through its
	// own credentials. Gateway association is not checked.
	//   "ASSOCIATION_AND_DEVICE_AUTH_TOKEN" - The device is authenticated
	// through both device credentials and gateway association. The device
	// must be bound to the gateway and must provide its own credentials.
	GatewayAuthMethod string `json:"gatewayAuthMethod,omitempty"`

	// GatewayType: Indicates whether the device is a gateway.
	//
	// Possible values:
	//   "GATEWAY_TYPE_UNSPECIFIED" - If unspecified, the device is
	// considered a non-gateway device.
	//   "GATEWAY" - The device is a gateway.
	//   "NON_GATEWAY" - The device is not a gateway.
	GatewayType string `json:"gatewayType,omitempty"`

	// LastAccessedGatewayId: [Output only] The ID of the gateway the device
	// accessed most recently.
	LastAccessedGatewayId string `json:"lastAccessedGatewayId,omitempty"`

	// LastAccessedGatewayTime: [Output only] The most recent time at which
	// the device accessed the gateway specified in `last_accessed_gateway`.
	LastAccessedGatewayTime string `json:"lastAccessedGatewayTime,omitempty"`

	// ForceSendFields is a list of field names (e.g. "GatewayAuthMethod")
	// to unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "GatewayAuthMethod") to
	// include in API requests with the JSON null value. By default, fields
	// with empty values are omitted from API requests. However, any field
	// with an empty value appearing in NullFields will be sent to the
	// server as null. It is an error if a field in this list has a
	// non-empty value. This may be used to include null fields in Patch
	// requests.
	NullFields []string `json:"-"`
}

func (s *GatewayConfig) MarshalJSON() ([]byte, error) {
	type NoMethod GatewayConfig
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// GetIamPolicyRequest: Request message for `GetIamPolicy` method.
type GetIamPolicyRequest struct {
	// Options: OPTIONAL: A `GetPolicyOptions` object for specifying options
	// to `GetIamPolicy`.
	Options *GetPolicyOptions `json:"options,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Options") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Options") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *GetIamPolicyRequest) MarshalJSON() ([]byte, error) {
	type NoMethod GetIamPolicyRequest
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// GetPolicyOptions: Encapsulates settings provided to GetIamPolicy.
type GetPolicyOptions struct {
	// RequestedPolicyVersion: Optional. The maximum policy version that
	// will be used to format the policy. Valid values are 0, 1, and 3.
	// Requests specifying an invalid value will be rejected. Requests for
	// policies with any conditional role bindings must specify version 3.
	// Policies with no conditional role bindings may specify any valid
	// value or leave the field unset. The policy in the response might use
	// the policy version that you specified, or it might use a lower policy
	// version. For example, if you specify version 3, but the policy has no
	// conditional role bindings, the response uses version 1. To learn
	// which resources support conditions in their IAM policies, see the IAM
	// documentation
	// (https://cloud.google.com/iam/help/conditions/resource-policies).
	RequestedPolicyVersion int64 `json:"requestedPolicyVersion,omitempty"`

	// ForceSendFields is a list of field names (e.g.
	// "RequestedPolicyVersion") to unconditionally include in API requests.
	// By default, fields with empty or default values are omitted from API
	// requests. However, any non-pointer, non-interface field appearing in
	// ForceSendFields will be sent to the server regardless of whether the
	// field is empty or not. This may be used to include empty fields in
	// Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "RequestedPolicyVersion")
	// to include in API requests with the JSON null value. By default,
	// fields with empty values are omitted from API requests. However, any
	// field with an empty value appearing in NullFields will be sent to the
	// server as null. It is an error if a field in this list has a
	// non-empty value. This may be used to include null fields in Patch
	// requests.
	NullFields []string `json:"-"`
}

func (s *GetPolicyOptions) MarshalJSON() ([]byte, error) {
	type NoMethod GetPolicyOptions
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// HttpConfig: The configuration of the HTTP bridge for a device
// registry.
type HttpConfig struct {
	// HttpEnabledState: If enabled, allows devices to use DeviceService via
	// the HTTP protocol. Otherwise, any requests to DeviceService will fail
	// for this registry.
	//
	// Possible values:
	//   "HTTP_STATE_UNSPECIFIED" - No HTTP state specified. If not
	// specified, DeviceService will be enabled by default.
	//   "HTTP_ENABLED" - Enables DeviceService (HTTP) service for the
	// registry.
	//   "HTTP_DISABLED" - Disables DeviceService (HTTP) service for the
	// registry.
	HttpEnabledState string `json:"httpEnabledState,omitempty"`

	// ForceSendFields is a list of field names (e.g. "HttpEnabledState") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "HttpEnabledState") to
	// include in API requests with the JSON null value. By default, fields
	// with empty values are omitted from API requests. However, any field
	// with an empty value appearing in NullFields will be sent to the
	// server as null. It is an error if a field in this list has a
	// non-empty value. This may be used to include null fields in Patch
	// requests.
	NullFields []string `json:"-"`
}

func (s *HttpConfig) MarshalJSON() ([]byte, error) {
	type NoMethod HttpConfig
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// ListDeviceConfigVersionsResponse: Response for
// `ListDeviceConfigVersions`.
type ListDeviceConfigVersionsResponse struct {
	// DeviceConfigs: The device configuration for the last few versions.
	// Versions are listed in decreasing order, starting from the most
	// recent one.
	DeviceConfigs []*DeviceConfig `json:"deviceConfigs,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "DeviceConfigs") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "DeviceConfigs") to include
	// in API requests with the JSON null value. By default, fields with
	// empty values are omitted from API requests. However, any field with
	// an empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *ListDeviceConfigVersionsResponse) MarshalJSON() ([]byte, error) {
	type NoMethod ListDeviceConfigVersionsResponse
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// ListDeviceRegistriesResponse: Response for `ListDeviceRegistries`.
type ListDeviceRegistriesResponse struct {
	// DeviceRegistries: The registries that matched the query.
	DeviceRegistries []*DeviceRegistry `json:"deviceRegistries,omitempty"`

	// NextPageToken: If not empty, indicates that there may be more
	// registries that match the request; this value should be passed in a
	// new `ListDeviceRegistriesRequest`.
	NextPageToken string `json:"nextPageToken,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "DeviceRegistries") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "DeviceRegistries") to
	// include in API requests with the JSON null value. By default, fields
	// with empty values are omitted from API requests. However, any field
	// with an empty value appearing in NullFields will be sent to the
	// server as null. It is an error if a field in this list has a
	// non-empty value. This may be used to include null fields in Patch
	// requests.
	NullFields []string `json:"-"`
}

func (s *ListDeviceRegistriesResponse) MarshalJSON() ([]byte, error) {
	type NoMethod ListDeviceRegistriesResponse
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// ListDeviceStatesResponse: Response for `ListDeviceStates`.
type ListDeviceStatesResponse struct {
	// DeviceStates: The last few device states. States are listed in
	// descending order of server update time, starting from the most recent
	// one.
	DeviceStates []*DeviceState `json:"deviceStates,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "DeviceStates") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "DeviceStates") to include
	// in API requests with the JSON null value. By default, fields with
	// empty values are omitted from API requests. However, any field with
	// an empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *ListDeviceStatesResponse) MarshalJSON() ([]byte, error) {
	type NoMethod ListDeviceStatesResponse
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// ListDevicesResponse: Response for `ListDevices`.
type ListDevicesResponse struct {
	// Devices: The devices that match the request.
	Devices []*Device `json:"devices,omitempty"`

	// NextPageToken: If not empty, indicates that there may be more devices
	// that match the request; this value should be passed in a new
	// `ListDevicesRequest`.
	NextPageToken string `json:"nextPageToken,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Devices") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Devices") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *ListDevicesResponse) MarshalJSON() ([]byte, error) {
	type NoMethod ListDevicesResponse
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// ModifyCloudToDeviceConfigRequest: Request for
// `ModifyCloudToDeviceConfig`.
type ModifyCloudToDeviceConfigRequest struct {
	// BinaryData: Required. The configuration data for the device.
	BinaryData string `json:"binaryData,omitempty"`

	// VersionToUpdate: The version number to update. If this value is zero,
	// it will not check the version number of the server and will always
	// update the current version; otherwise, this update will fail if the
	// version number found on the server does not match this version
	// number. This is used to support multiple simultaneous updates without
	// losing data.
	VersionToUpdate int64 `json:"versionToUpdate,omitempty,string"`

	// ForceSendFields is a list of field names (e.g. "BinaryData") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "BinaryData") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *ModifyCloudToDeviceConfigRequest) MarshalJSON() ([]byte, error) {
	type NoMethod ModifyCloudToDeviceConfigRequest
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// MqttConfig: The configuration of MQTT for a device registry.
type MqttConfig struct {
	// MqttEnabledState: If enabled, allows connections using the MQTT
	// protocol. Otherwise, MQTT connections to this registry will fail.
	//
	// Possible values:
	//   "MQTT_STATE_UNSPECIFIED" - No MQTT state specified. If not
	// specified, MQTT will be enabled by default.
	//   "MQTT_ENABLED" - Enables a MQTT connection.
	//   "MQTT_DISABLED" - Disables a MQTT connection.
	MqttEnabledState string `json:"mqttEnabledState,omitempty"`

	// ForceSendFields is a list of field names (e.g. "MqttEnabledState") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "MqttEnabledState") to
	// include in API requests with the JSON null value. By default, fields
	// with empty values are omitted from API requests. However, any field
	// with an empty value appearing in NullFields will be sent to the
	// server as null. It is an error if a field in this list has a
	// non-empty value. This may be used to include null fields in Patch
	// requests.
	NullFields []string `json:"-"`
}

func (s *MqttConfig) MarshalJSON() ([]byte, error) {
	type NoMethod MqttConfig
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// Policy: An Identity and Access Management (IAM) policy, which
// specifies access controls for Google Cloud resources. A `Policy` is a
// collection of `bindings`. A `binding` binds one or more `members`, or
// principals, to a single `role`. Principals can be user accounts,
// service accounts, Google groups, and domains (such as G Suite). A
// `role` is a named list of permissions; each `role` can be an IAM
// predefined role or a user-created custom role. For some types of
// Google Cloud resources, a `binding` can also specify a `condition`,
// which is a logical expression that allows access to a resource only
// if the expression evaluates to `true`. A condition can add
// constraints based on attributes of the request, the resource, or
// both. To learn which resources support conditions in their IAM
// policies, see the IAM documentation
// (https://cloud.google.com/iam/help/conditions/resource-policies).
// **JSON example:** { "bindings": [ { "role":
// "roles/resourcemanager.organizationAdmin", "members": [
// "user:mike@example.com", "group:admins@example.com",
// "domain:google.com",
// "serviceAccount:my-project-id@appspot.gserviceaccount.com" ] }, {
// "role": "roles/resourcemanager.organizationViewer", "members": [
// "user:eve@example.com" ], "condition": { "title": "expirable access",
// "description": "Does not grant access after Sep 2020", "expression":
// "request.time < timestamp('2020-10-01T00:00:00.000Z')", } } ],
// "etag": "BwWWja0YfJA=", "version": 3 } **YAML example:** bindings: -
// members: - user:mike@example.com - group:admins@example.com -
// domain:google.com -
// serviceAccount:my-project-id@appspot.gserviceaccount.com role:
// roles/resourcemanager.organizationAdmin - members: -
// user:eve@example.com role: roles/resourcemanager.organizationViewer
// condition: title: expirable access description: Does not grant access
// after Sep 2020 expression: request.time <
// timestamp('2020-10-01T00:00:00.000Z') etag: BwWWja0YfJA= version: 3
// For a description of IAM and its features, see the IAM documentation
// (https://cloud.google.com/iam/docs/).
type Policy struct {
	// Bindings: Associates a list of `members`, or principals, with a
	// `role`. Optionally, may specify a `condition` that determines how and
	// when the `bindings` are applied. Each of the `bindings` must contain
	// at least one principal. The `bindings` in a `Policy` can refer to up
	// to 1,500 principals; up to 250 of these principals can be Google
	// groups. Each occurrence of a principal counts towards these limits.
	// For example, if the `bindings` grant 50 different roles to
	// `user:alice@example.com`, and not to any other principal, then you
	// can add another 1,450 principals to the `bindings` in the `Policy`.
	Bindings []*Binding `json:"bindings,omitempty"`

	// Etag: `etag` is used for optimistic concurrency control as a way to
	// help prevent simultaneous updates of a policy from overwriting each
	// other. It is strongly suggested that systems make use of the `etag`
	// in the read-modify-write cycle to perform policy updates in order to
	// avoid race conditions: An `etag` is returned in the response to
	// `getIamPolicy`, and systems are expected to put that etag in the
	// request to `setIamPolicy` to ensure that their change will be applied
	// to the same version of the policy. **Important:** If you use IAM
	// Conditions, you must include the `etag` field whenever you call
	// `setIamPolicy`. If you omit this field, then IAM allows you to
	// overwrite a version `3` policy with a version `1` policy, and all of
	// the conditions in the version `3` policy are lost.
	Etag string `json:"etag,omitempty"`

	// Version: Specifies the format of the policy. Valid values are `0`,
	// `1`, and `3`. Requests that specify an invalid value are rejected.
	// Any operation that affects conditional role bindings must specify
	// version `3`. This requirement applies to the following operations: *
	// Getting a policy that includes a conditional role binding * Adding a
	// conditional role binding to a policy * Changing a conditional role
	// binding in a policy * Removing any role binding, with or without a
	// condition, from a policy that includes conditions **Important:** If
	// you use IAM Conditions, you must include the `etag` field whenever
	// you call `setIamPolicy`. If you omit this field, then IAM allows you
	// to overwrite a version `3` policy with a version `1` policy, and all
	// of the conditions in the version `3` policy are lost. If a policy
	// does not include any conditions, operations on that policy may
	// specify any valid version or leave the field unset. To learn which
	// resources support conditions in their IAM policies, see the IAM
	// documentation
	// (https://cloud.google.com/iam/help/conditions/resource-policies).
	Version int64 `json:"version,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Bindings") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Bindings") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *Policy) MarshalJSON() ([]byte, error) {
	type NoMethod Policy
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// PublicKeyCertificate: A public key certificate format and data.
type PublicKeyCertificate struct {
	// Certificate: The certificate data.
	Certificate string `json:"certificate,omitempty"`

	// Format: The certificate format.
	//
	// Possible values:
	//   "UNSPECIFIED_PUBLIC_KEY_CERTIFICATE_FORMAT" - The format has not
	// been specified. This is an invalid default value and must not be
	// used.
	//   "X509_CERTIFICATE_PEM" - An X.509v3 certificate
	// ([RFC5280](https://www.ietf.org/rfc/rfc5280.txt)), encoded in base64,
	// and wrapped by `-----BEGIN CERTIFICATE-----` and `-----END
	// CERTIFICATE-----`.
	Format string `json:"format,omitempty"`

	// X509Details: [Output only] The certificate details. Used only for
	// X.509 certificates.
	X509Details *X509CertificateDetails `json:"x509Details,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Certificate") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Certificate") to include
	// in API requests with the JSON null value. By default, fields with
	// empty values are omitted from API requests. However, any field with
	// an empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *PublicKeyCertificate) MarshalJSON() ([]byte, error) {
	type NoMethod PublicKeyCertificate
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// PublicKeyCredential: A public key format and data.
type PublicKeyCredential struct {
	// Format: The format of the key.
	//
	// Possible values:
	//   "UNSPECIFIED_PUBLIC_KEY_FORMAT" - The format has not been
	// specified. This is an invalid default value and must not be used.
	//   "RSA_PEM" - An RSA public key encoded in base64, and wrapped by
	// `-----BEGIN PUBLIC KEY-----` and `-----END PUBLIC KEY-----`. This can
	// be used to verify `RS256` signatures in JWT tokens ([RFC7518](
	// https://www.ietf.org/rfc/rfc7518.txt)).
	//   "RSA_X509_PEM" - As RSA_PEM, but wrapped in an X.509v3 certificate
	// ([RFC5280]( https://www.ietf.org/rfc/rfc5280.txt)), encoded in
	// base64, and wrapped by `-----BEGIN CERTIFICATE-----` and `-----END
	// CERTIFICATE-----`.
	//   "ES256_PEM" - Public key for the ECDSA algorithm using P-256 and
	// SHA-256, encoded in base64, and wrapped by `-----BEGIN PUBLIC
	// KEY-----` and `-----END PUBLIC KEY-----`. This can be used to verify
	// JWT tokens with the `ES256` algorithm
	// ([RFC7518](https://www.ietf.org/rfc/rfc7518.txt)). This curve is
	// defined in [OpenSSL](https://www.openssl.org/) as the `prime256v1`
	// curve.
	//   "ES256_X509_PEM" - As ES256_PEM, but wrapped in an X.509v3
	// certificate ([RFC5280]( https://www.ietf.org/rfc/rfc5280.txt)),
	// encoded in base64, and wrapped by `-----BEGIN CERTIFICATE-----` and
	// `-----END CERTIFICATE-----`.
	Format string `json:"format,omitempty"`

	// Key: The key data.
	Key string `json:"key,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Format") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Format") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *PublicKeyCredential) MarshalJSON() ([]byte, error) {
	type NoMethod PublicKeyCredential
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// RegistryCredential: A server-stored registry credential used to
// validate device credentials.
type RegistryCredential struct {
	// PublicKeyCertificate: A public key certificate used to verify the
	// device credentials.
	PublicKeyCertificate *PublicKeyCertificate `json:"publicKeyCertificate,omitempty"`

	// ForceSendFields is a list of field names (e.g.
	// "PublicKeyCertificate") to unconditionally include in API requests.
	// By default, fields with empty or default values are omitted from API
	// requests. However, any non-pointer, non-interface field appearing in
	// ForceSendFields will be sent to the server regardless of whether the
	// field is empty or not. This may be used to include empty fields in
	// Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "PublicKeyCertificate") to
	// include in API requests with the JSON null value. By default, fields
	// with empty values are omitted from API requests. However, any field
	// with an empty value appearing in NullFields will be sent to the
	// server as null. It is an error if a field in this list has a
	// non-empty value. This may be used to include null fields in Patch
	// requests.
	NullFields []string `json:"-"`
}

func (s *RegistryCredential) MarshalJSON() ([]byte, error) {
	type NoMethod RegistryCredential
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SendCommandToDeviceRequest: Request for `SendCommandToDevice`.
type SendCommandToDeviceRequest struct {
	// BinaryData: Required. The command data to send to the device.
	BinaryData string `json:"binaryData,omitempty"`

	// Subfolder: Optional subfolder for the command. If empty, the command
	// will be delivered to the /devices/{device-id}/commands topic,
	// otherwise it will be delivered to the
	// /devices/{device-id}/commands/{subfolder} topic. Multi-level
	// subfolders are allowed. This field must not have more than 256
	// characters, and must not contain any MQTT wildcards ("+" or "#") or
	// null characters.
	Subfolder string `json:"subfolder,omitempty"`

	// ForceSendFields is a list of field names (e.g. "BinaryData") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "BinaryData") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SendCommandToDeviceRequest) MarshalJSON() ([]byte, error) {
	type NoMethod SendCommandToDeviceRequest
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SendCommandToDeviceResponse: Response for `SendCommandToDevice`.
type SendCommandToDeviceResponse struct {
	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`
}

// SetIamPolicyRequest: Request message for `SetIamPolicy` method.
type SetIamPolicyRequest struct {
	// Policy: REQUIRED: The complete policy to be applied to the
	// `resource`. The size of the policy is limited to a few 10s of KB. An
	// empty policy is a valid policy but certain Google Cloud services
	// (such as Projects) might reject them.
	Policy *Policy `json:"policy,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Policy") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Policy") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SetIamPolicyRequest) MarshalJSON() ([]byte, error) {
	type NoMethod SetIamPolicyRequest
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// StateNotificationConfig: The configuration for notification of new
// states received from the device.
type StateNotificationConfig struct {
	// PubsubTopicName: A Cloud Pub/Sub topic name. For example,
	// `projects/myProject/topics/deviceEvents`.
	PubsubTopicName string `json:"pubsubTopicName,omitempty"`

	// ForceSendFields is a list of field names (e.g. "PubsubTopicName") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "PubsubTopicName") to
	// include in API requests with the JSON null value. By default, fields
	// with empty values are omitted from API requests. However, any field
	// with an empty value appearing in NullFields will be sent to the
	// server as null. It is an error if a field in this list has a
	// non-empty value. This may be used to include null fields in Patch
	// requests.
	NullFields []string `json:"-"`
}

func (s *StateNotificationConfig) MarshalJSON() ([]byte, error) {
	type NoMethod StateNotificationConfig
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// Status: The `Status` type defines a logical error model that is
// suitable for different programming environments, including REST APIs
// and RPC APIs. It is used by gRPC (https://github.com/grpc). Each
// `Status` message contains three pieces of data: error code, error
// message, and error details. You can find out more about this error
// model and how to work with it in the API Design Guide
// (https://cloud.google.com/apis/design/errors).
type Status struct {
	// Code: The status code, which should be an enum value of
	// google.rpc.Code.
	Code int64 `json:"code,omitempty"`

	// Details: A list of messages that carry the error details. There is a
	// common set of message types for APIs to use.
	Details []googleapi.RawMessage `json:"details,omitempty"`

	// Message: A developer-facing error message, which should be in
	// English. Any user-facing error message should be localized and sent
	// in the google.rpc.Status.details field, or localized by the client.
	Message string `json:"message,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Code") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Code") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *Status) MarshalJSON() ([]byte, error) {
	type NoMethod Status
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// TestIamPermissionsRequest: Request message for `TestIamPermissions`
// method.
type TestIamPermissionsRequest struct {
	// Permissions: The set of permissions to check for the `resource`.
	// Permissions with wildcards (such as `*` or `storage.*`) are not
	// allowed. For more information see IAM Overview
	// (https://cloud.google.com/iam/docs/overview#permissions).
	Permissions []string `json:"permissions,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Permissions") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Permissions") to include
	// in API requests with the JSON null value. By default, fields with
	// empty values are omitted from API requests. However, any field with
	// an empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *TestIamPermissionsRequest) MarshalJSON() ([]byte, error) {
	type NoMethod TestIamPermissionsRequest
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// TestIamPermissionsResponse: Response message for `TestIamPermissions`
// method.
type TestIamPermissionsResponse struct {
	// Permissions: A subset of `TestPermissionsRequest.permissions` that
	// the caller is allowed.
	Permissions []string `json:"permissions,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Permissions") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Permissions") to include
	// in API requests with the JSON null value. By default, fields with
	// empty values are omitted from API requests. However, any field with
	// an empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *TestIamPermissionsResponse) MarshalJSON() ([]byte, error) {
	type NoMethod TestIamPermissionsResponse
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// UnbindDeviceFromGatewayRequest: Request for
// `UnbindDeviceFromGateway`.
type UnbindDeviceFromGatewayRequest struct {
	// DeviceId: Required. The device to disassociate from the specified
	// gateway. The value of `device_id` can be either the device numeric ID
	// or the user-defined device identifier.
	DeviceId string `json:"deviceId,omitempty"`

	// GatewayId: Required. The value of `gateway_id` can be either the
	// device numeric ID or the user-defined device identifier.
	GatewayId string `json:"gatewayId,omitempty"`

	// ForceSendFields is a list of field names (e.g. "DeviceId") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "DeviceId") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *UnbindDeviceFromGatewayRequest) MarshalJSON() ([]byte, error) {
	type NoMethod UnbindDeviceFromGatewayRequest
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// UnbindDeviceFromGatewayResponse: Response for
// `UnbindDeviceFromGateway`.
type UnbindDeviceFromGatewayResponse struct {
	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`
}

// X509CertificateDetails: Details of an X.509 certificate. For
// informational purposes only.
type X509CertificateDetails struct {
	// ExpiryTime: The time the certificate becomes invalid.
	ExpiryTime string `json:"expiryTime,omitempty"`

	// Issuer: The entity that signed the certificate.
	Issuer string `json:"issuer,omitempty"`

	// PublicKeyType: The type of public key in the certificate.
	PublicKeyType string `json:"publicKeyType,omitempty"`

	// SignatureAlgorithm: The algorithm used to sign the certificate.
	SignatureAlgorithm string `json:"signatureAlgorithm,omitempty"`

	// StartTime: The time the certificate becomes valid.
	StartTime string `json:"startTime,omitempty"`

	// Subject: The entity the certificate and public key belong to.
	Subject string `json:"subject,omitempty"`

	// ForceSendFields is a list of field names (e.g. "ExpiryTime") to
	// unconditionally include in API requests. By default, fields with
	// empty or default values are omitted from API requests. However, any
	// non-pointer, non-interface field appearing in ForceSendFields will be
	// sent to the server regardless of whether the field is empty or not.
	// This may be used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "ExpiryTime") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *X509CertificateDetails) MarshalJSON() ([]byte, error) {
	type NoMethod X509CertificateDetails
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// method id "cloudiot.projects.locations.registries.bindDeviceToGateway":

type ProjectsLocationsRegistriesBindDeviceToGatewayCall struct {
	s                          *Service
	parent                     string
	binddevicetogatewayrequest *BindDeviceToGatewayRequest
	urlParams_                 gensupport.URLParams
	ctx_                       context.Context
	header_                    http.Header
}

// BindDeviceToGateway: Associates the device with the gateway.
//
//   - parent: The name of the registry. For example,
//     `projects/example-project/locations/us-central1/registries/my-regist
//     ry`.
func (r *ProjectsLocationsRegistriesService) BindDeviceToGateway(parent string, binddevicetogatewayrequest *BindDeviceToGatewayRequest) *ProjectsLocationsRegistriesBindDeviceToGatewayCall {
	c := &ProjectsLocationsRegistriesBindDeviceToGatewayCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.parent = parent
	c.binddevicetogatewayrequest = binddevicetogatewayrequest
	c.urlParams_.Set("method", "bindDeviceToGateway")
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesBindDeviceToGatewayCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesBindDeviceToGatewayCall {
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesBindDeviceToGatewayCall) Context(ctx context.Context) *ProjectsLocationsRegistriesBindDeviceToGatewayCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesBindDeviceToGatewayCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesBindDeviceToGatewayCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.binddevicetogatewayrequest)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")
	matches, err := c.s.TemplatePaths.RegistryPathTemplate.Match(c.parent)
	if err != nil {
		return nil, err
	}
	registry := matches["registry"]
	location := matches["location"]
	credentials := GetRegistryCredentials(registry, location, c.s)
	reqHeaders.Set("ClearBlade-UserToken", credentials.Token)

	urls := fmt.Sprintf("%s/api/v/4/webhook/execute/%s/cloudiot", credentials.Url, credentials.SystemKey)
	urls += "?" + c.urlParams_.Encode()
	req, err := http.NewRequest("POST", urls, body)
	if err != nil {
		return nil, err
	}
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"parent": c.parent,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.bindDeviceToGateway" call.
// Exactly one of *BindDeviceToGatewayResponse or error will be non-nil.
// Any non-2xx status code is an error. Response headers are in either
// *BindDeviceToGatewayResponse.ServerResponse.Header or (if a response
// was returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ProjectsLocationsRegistriesBindDeviceToGatewayCall) Do() (*BindDeviceToGatewayResponse, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &BindDeviceToGatewayResponse{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Associates the device with the gateway.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}:bindDeviceToGateway",
	//   "httpMethod": "POST",
	//   "id": "cloudiot.projects.locations.registries.bindDeviceToGateway",
	//   "parameterOrder": [
	//     "parent"
	//   ],
	//   "parameters": {
	//     "parent": {
	//       "description": "Required. The name of the registry. For example, `projects/example-project/locations/us-central1/registries/my-registry`.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+parent}:bindDeviceToGateway",
	//   "request": {
	//     "$ref": "BindDeviceToGatewayRequest"
	//   },
	//   "response": {
	//     "$ref": "BindDeviceToGatewayResponse"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// method id "cloudiot.projects.locations.registries.create":

type ProjectsLocationsRegistriesCreateCall struct {
	s              *Service
	parent         string
	deviceregistry *DeviceRegistry
	urlParams_     gensupport.URLParams
	ctx_           context.Context
	header_        http.Header
}

// Create: Creates a device registry that contains devices.
//
//   - parent: The project and cloud region where this device registry
//     must be created. For example,
//     `projects/example-project/locations/us-central1`.
func (r *ProjectsLocationsRegistriesService) Create(parent string, deviceregistry *DeviceRegistry) *ProjectsLocationsRegistriesCreateCall {
	c := &ProjectsLocationsRegistriesCreateCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.parent = parent
	c.urlParams_.Set("parent", parent)
	c.deviceregistry = deviceregistry
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesCreateCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesCreateCall {
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesCreateCall) Context(ctx context.Context) *ProjectsLocationsRegistriesCreateCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesCreateCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesCreateCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.deviceregistry)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")
	reqHeaders.Set("ClearBlade-UserToken", c.s.ServiceAccountCredentials.Token)

	urls := fmt.Sprintf("%s/api/v/4/webhook/execute/%s/cloudiot", c.s.ServiceAccountCredentials.Url, c.s.ServiceAccountCredentials.SystemKey)
	urls += "?" + c.urlParams_.Encode()
	req, err := http.NewRequest("POST", urls, body)
	if err != nil {
		return nil, err
	}
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"parent": c.parent,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.create" call.
// Exactly one of *DeviceRegistry or error will be non-nil. Any non-2xx
// status code is an error. Response headers are in either
// *DeviceRegistry.ServerResponse.Header or (if a response was returned
// at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ProjectsLocationsRegistriesCreateCall) Do() (*DeviceRegistry, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &DeviceRegistry{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Creates a device registry that contains devices.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries",
	//   "httpMethod": "POST",
	//   "id": "cloudiot.projects.locations.registries.create",
	//   "parameterOrder": [
	//     "parent"
	//   ],
	//   "parameters": {
	//     "parent": {
	//       "description": "Required. The project and cloud region where this device registry must be created. For example, `projects/example-project/locations/us-central1`.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+parent}/registries",
	//   "request": {
	//     "$ref": "DeviceRegistry"
	//   },
	//   "response": {
	//     "$ref": "DeviceRegistry"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// method id "cloudiot.projects.locations.registries.delete":

type ProjectsLocationsRegistriesDeleteCall struct {
	s          *Service
	name       string
	urlParams_ gensupport.URLParams
	ctx_       context.Context
	header_    http.Header
}

// Delete: Deletes a device registry configuration.
//
//   - name: The name of the device registry. For example,
//     `projects/example-project/locations/us-central1/registries/my-regist
//     ry`.
func (r *ProjectsLocationsRegistriesService) Delete(name string) *ProjectsLocationsRegistriesDeleteCall {
	c := &ProjectsLocationsRegistriesDeleteCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.name = name
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesDeleteCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesDeleteCall {
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesDeleteCall) Context(ctx context.Context) *ProjectsLocationsRegistriesDeleteCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesDeleteCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesDeleteCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	var body io.Reader = nil
	reqHeaders.Set("ClearBlade-UserToken", c.s.ServiceAccountCredentials.Token)

	urls := fmt.Sprintf("%s/api/v/4/webhook/execute/%s/cloudiot", c.s.ServiceAccountCredentials.Url, c.s.ServiceAccountCredentials.SystemKey)
	urls += "?" + c.urlParams_.Encode()
	req, err := http.NewRequest("DELETE", urls, body)
	if err != nil {
		return nil, err
	}
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"name": c.name,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.delete" call.
// Exactly one of *Empty or error will be non-nil. Any non-2xx status
// code is an error. Response headers are in either
// *Empty.ServerResponse.Header or (if a response was returned at all)
// in error.(*googleapi.Error).Header. Use googleapi.IsNotModified to
// check whether the returned error was because http.StatusNotModified
// was returned.
func (c *ProjectsLocationsRegistriesDeleteCall) Do() (*Empty, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &Empty{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	return ret, nil
	// {
	//   "description": "Deletes a device registry configuration.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}",
	//   "httpMethod": "DELETE",
	//   "id": "cloudiot.projects.locations.registries.delete",
	//   "parameterOrder": [
	//     "name"
	//   ],
	//   "parameters": {
	//     "name": {
	//       "description": "Required. The name of the device registry. For example, `projects/example-project/locations/us-central1/registries/my-registry`.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+name}",
	//   "response": {
	//     "$ref": "Empty"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// method id "cloudiot.projects.locations.registries.get":

type ProjectsLocationsRegistriesGetCall struct {
	s            *Service
	name         string
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// Get: Gets a device registry configuration.
//
//   - name: The name of the device registry. For example,
//     `projects/example-project/locations/us-central1/registries/my-regist
//     ry`.
func (r *ProjectsLocationsRegistriesService) Get(name string) *ProjectsLocationsRegistriesGetCall {
	c := &ProjectsLocationsRegistriesGetCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.name = name
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesGetCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesGetCall {
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *ProjectsLocationsRegistriesGetCall) IfNoneMatch(entityTag string) *ProjectsLocationsRegistriesGetCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesGetCall) Context(ctx context.Context) *ProjectsLocationsRegistriesGetCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesGetCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesGetCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil

	matches, err := c.s.TemplatePaths.RegistryPathTemplate.Match(c.name)
	if err != nil {
		return nil, err
	}
	registry := matches["registry"]
	location := matches["location"]
	credentials := GetRegistryCredentials(registry, location, c.s)
	reqHeaders.Set("ClearBlade-UserToken", credentials.Token)

	urls := fmt.Sprintf("%s/api/v/4/webhook/execute/%s/cloudiot", credentials.Url, credentials.SystemKey)
	urls += "?" + c.urlParams_.Encode()
	req, err := http.NewRequest("GET", urls, body)
	if err != nil {
		return nil, err
	}
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"name": c.name,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.get" call.
// Exactly one of *DeviceRegistry or error will be non-nil. Any non-2xx
// status code is an error. Response headers are in either
// *DeviceRegistry.ServerResponse.Header or (if a response was returned
// at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ProjectsLocationsRegistriesGetCall) Do() (*DeviceRegistry, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &DeviceRegistry{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Gets a device registry configuration.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}",
	//   "httpMethod": "GET",
	//   "id": "cloudiot.projects.locations.registries.get",
	//   "parameterOrder": [
	//     "name"
	//   ],
	//   "parameters": {
	//     "name": {
	//       "description": "Required. The name of the device registry. For example, `projects/example-project/locations/us-central1/registries/my-registry`.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+name}",
	//   "response": {
	//     "$ref": "DeviceRegistry"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// method id "cloudiot.projects.locations.registries.getIamPolicy":

type ProjectsLocationsRegistriesGetIamPolicyCall struct {
	s                   *Service
	resource            string
	getiampolicyrequest *GetIamPolicyRequest
	urlParams_          gensupport.URLParams
	ctx_                context.Context
	header_             http.Header
}

// GetIamPolicy: Gets the access control policy for a resource. Returns
// an empty policy if the resource exists and does not have a policy
// set.
//
//   - resource: REQUIRED: The resource for which the policy is being
//     requested. See Resource names
//     (https://cloud.google.com/apis/design/resource_names) for the
//     appropriate value for this field.
func (r *ProjectsLocationsRegistriesService) GetIamPolicy(resource string, getiampolicyrequest *GetIamPolicyRequest) *ProjectsLocationsRegistriesGetIamPolicyCall {
	c := &ProjectsLocationsRegistriesGetIamPolicyCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.resource = resource
	c.getiampolicyrequest = getiampolicyrequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesGetIamPolicyCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesGetIamPolicyCall {
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesGetIamPolicyCall) Context(ctx context.Context) *ProjectsLocationsRegistriesGetIamPolicyCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesGetIamPolicyCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesGetIamPolicyCall) doRequest(alt string) (*http.Response, error) {
	return nil, errors.New("Not implemented")
	// reqHeaders := make(http.Header)
	// for k, v := range c.header_ {
	// 	reqHeaders[k] = v
	// }
	// var body io.Reader = nil
	// body, err := googleapi.WithoutDataWrapper.JSONReader(c.getiampolicyrequest)
	// if err != nil {
	// 	return nil, err
	// }
	// reqHeaders.Set("Content-Type", "application/json")
	// urls := googleapi.ResolveRelative(c.s.ServiceAccountCredentials.Url, "v1/{+resource}:getIamPolicy")
	// urls += "?" + c.urlParams_.Encode()
	// req, err := http.NewRequest("POST", urls, body)
	// if err != nil {
	// 	return nil, err
	// }
	// req.Header = reqHeaders
	// googleapi.Expand(req.URL, map[string]string{
	// 	"resource": c.resource,
	// })
	// return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.getIamPolicy" call.
// Exactly one of *Policy or error will be non-nil. Any non-2xx status
// code is an error. Response headers are in either
// *Policy.ServerResponse.Header or (if a response was returned at all)
// in error.(*googleapi.Error).Header. Use googleapi.IsNotModified to
// check whether the returned error was because http.StatusNotModified
// was returned.
func (c *ProjectsLocationsRegistriesGetIamPolicyCall) Do() (*Policy, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &Policy{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Gets the access control policy for a resource. Returns an empty policy if the resource exists and does not have a policy set.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}:getIamPolicy",
	//   "httpMethod": "POST",
	//   "id": "cloudiot.projects.locations.registries.getIamPolicy",
	//   "parameterOrder": [
	//     "resource"
	//   ],
	//   "parameters": {
	//     "resource": {
	//       "description": "REQUIRED: The resource for which the policy is being requested. See [Resource names](https://cloud.google.com/apis/design/resource_names) for the appropriate value for this field.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+resource}:getIamPolicy",
	//   "request": {
	//     "$ref": "GetIamPolicyRequest"
	//   },
	//   "response": {
	//     "$ref": "Policy"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// method id "cloudiot.projects.locations.registries.list":

type ProjectsLocationsRegistriesListCall struct {
	s            *Service
	parent       string
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// List: Lists device registries.
//
//   - parent: The project and cloud region path. For example,
//     `projects/example-project/locations/us-central1`.
func (r *ProjectsLocationsRegistriesService) List(parent string) *ProjectsLocationsRegistriesListCall {
	c := &ProjectsLocationsRegistriesListCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.parent = parent
	c.urlParams_.Set("parent", parent)
	return c
}

// PageSize sets the optional parameter "pageSize": The maximum number
// of registries to return in the response. If this value is zero, the
// service will select a default size. A call may return fewer objects
// than requested. A non-empty `next_page_token` in the response
// indicates that more data is available.
func (c *ProjectsLocationsRegistriesListCall) PageSize(pageSize int64) *ProjectsLocationsRegistriesListCall {
	c.urlParams_.Set("pageSize", fmt.Sprint(pageSize))
	return c
}

// PageToken sets the optional parameter "pageToken": The value returned
// by the last `ListDeviceRegistriesResponse`; indicates that this is a
// continuation of a prior `ListDeviceRegistries` call and the system
// should return the next page of data.
func (c *ProjectsLocationsRegistriesListCall) PageToken(pageToken string) *ProjectsLocationsRegistriesListCall {
	c.urlParams_.Set("pageToken", pageToken)
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesListCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesListCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *ProjectsLocationsRegistriesListCall) IfNoneMatch(entityTag string) *ProjectsLocationsRegistriesListCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesListCall) Context(ctx context.Context) *ProjectsLocationsRegistriesListCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesListCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesListCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	reqHeaders.Set("ClearBlade-UserToken", c.s.ServiceAccountCredentials.Token)

	urls := fmt.Sprintf("%s/api/v/4/webhook/execute/%s/cloudiot", c.s.ServiceAccountCredentials.Url, c.s.ServiceAccountCredentials.SystemKey)
	urls += "?" + c.urlParams_.Encode()
	req, err := http.NewRequest("GET", urls, body)
	if err != nil {
		return nil, err
	}
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"parent": c.parent,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.list" call.
// Exactly one of *ListDeviceRegistriesResponse or error will be
// non-nil. Any non-2xx status code is an error. Response headers are in
// either *ListDeviceRegistriesResponse.ServerResponse.Header or (if a
// response was returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ProjectsLocationsRegistriesListCall) Do() (*ListDeviceRegistriesResponse, error) {
	res, err := c.doRequest("json")
	bodybytes, err := io.ReadAll(res.Body)
	fmt.Printf("res: %s\n", string(bodybytes))
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &ListDeviceRegistriesResponse{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Lists device registries.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries",
	//   "httpMethod": "GET",
	//   "id": "cloudiot.projects.locations.registries.list",
	//   "parameterOrder": [
	//     "parent"
	//   ],
	//   "parameters": {
	//     "pageSize": {
	//       "description": "The maximum number of registries to return in the response. If this value is zero, the service will select a default size. A call may return fewer objects than requested. A non-empty `next_page_token` in the response indicates that more data is available.",
	//       "format": "int32",
	//       "location": "query",
	//       "type": "integer"
	//     },
	//     "pageToken": {
	//       "description": "The value returned by the last `ListDeviceRegistriesResponse`; indicates that this is a continuation of a prior `ListDeviceRegistries` call and the system should return the next page of data.",
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "parent": {
	//       "description": "Required. The project and cloud region path. For example, `projects/example-project/locations/us-central1`.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+parent}/registries",
	//   "response": {
	//     "$ref": "ListDeviceRegistriesResponse"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// Pages invokes f for each page of results.
// A non-nil error returned from f will halt the iteration.
// The provided context supersedes any context provided to the Context method.
func (c *ProjectsLocationsRegistriesListCall) Pages(ctx context.Context, f func(*ListDeviceRegistriesResponse) error) error {
	c.ctx_ = ctx
	defer c.PageToken(c.urlParams_.Get("pageToken")) // reset paging to original point
	for {
		x, err := c.Do()
		if err != nil {
			return err
		}
		if err := f(x); err != nil {
			return err
		}
		if x.NextPageToken == "" {
			return nil
		}
		c.PageToken(x.NextPageToken)
	}
}

// method id "cloudiot.projects.locations.registries.patch":

type ProjectsLocationsRegistriesPatchCall struct {
	s              *Service
	name           string
	deviceregistry *DeviceRegistry
	urlParams_     gensupport.URLParams
	ctx_           context.Context
	header_        http.Header
}

// Patch: Updates a device registry configuration.
//
//   - name: The resource path name. For example,
//     `projects/example-project/locations/us-central1/registries/my-regist
//     ry`.
func (r *ProjectsLocationsRegistriesService) Patch(name string, deviceregistry *DeviceRegistry) *ProjectsLocationsRegistriesPatchCall {
	c := &ProjectsLocationsRegistriesPatchCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.name = name
	c.urlParams_.Set("name", c.name)
	c.deviceregistry = deviceregistry
	return c
}

// UpdateMask sets the optional parameter "updateMask": Required. Only
// updates the `device_registry` fields indicated by this mask. The
// field mask must not be empty, and it must not contain fields that are
// immutable or only set by the server. Mutable top-level fields:
// `event_notification_config`, `http_config`, `mqtt_config`, and
// `state_notification_config`.
func (c *ProjectsLocationsRegistriesPatchCall) UpdateMask(updateMask string) *ProjectsLocationsRegistriesPatchCall {
	c.urlParams_.Set("updateMask", updateMask)
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesPatchCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesPatchCall {
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesPatchCall) Context(ctx context.Context) *ProjectsLocationsRegistriesPatchCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesPatchCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesPatchCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.deviceregistry)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")

	matches, err := c.s.TemplatePaths.RegistryPathTemplate.Match(c.name)
	if err != nil {
		return nil, err
	}

	registry := matches["registry"]
	location := matches["location"]
	credentials := GetRegistryCredentials(registry, location, c.s)
	reqHeaders.Set("ClearBlade-UserToken", credentials.Token)

	urls := fmt.Sprintf("%s/api/v/4/webhook/execute/%s/cloudiot", credentials.Url, credentials.SystemKey)
	urls += "?" + c.urlParams_.Encode()
	req, err := http.NewRequest("PATCH", urls, body)
	if err != nil {
		return nil, err
	}
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"name": c.name,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.patch" call.
// Exactly one of *DeviceRegistry or error will be non-nil. Any non-2xx
// status code is an error. Response headers are in either
// *DeviceRegistry.ServerResponse.Header or (if a response was returned
// at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ProjectsLocationsRegistriesPatchCall) Do() (*DeviceRegistry, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &DeviceRegistry{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Updates a device registry configuration.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}",
	//   "httpMethod": "PATCH",
	//   "id": "cloudiot.projects.locations.registries.patch",
	//   "parameterOrder": [
	//     "name"
	//   ],
	//   "parameters": {
	//     "name": {
	//       "description": "The resource path name. For example, `projects/example-project/locations/us-central1/registries/my-registry`.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     },
	//     "updateMask": {
	//       "description": "Required. Only updates the `device_registry` fields indicated by this mask. The field mask must not be empty, and it must not contain fields that are immutable or only set by the server. Mutable top-level fields: `event_notification_config`, `http_config`, `mqtt_config`, and `state_notification_config`.",
	//       "format": "google-fieldmask",
	//       "location": "query",
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+name}",
	//   "request": {
	//     "$ref": "DeviceRegistry"
	//   },
	//   "response": {
	//     "$ref": "DeviceRegistry"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// method id "cloudiot.projects.locations.registries.setIamPolicy":

type ProjectsLocationsRegistriesSetIamPolicyCall struct {
	s                   *Service
	resource            string
	setiampolicyrequest *SetIamPolicyRequest
	urlParams_          gensupport.URLParams
	ctx_                context.Context
	header_             http.Header
}

// SetIamPolicy: Sets the access control policy on the specified
// resource. Replaces any existing policy.
//
//   - resource: REQUIRED: The resource for which the policy is being
//     specified. See Resource names
//     (https://cloud.google.com/apis/design/resource_names) for the
//     appropriate value for this field.
func (r *ProjectsLocationsRegistriesService) SetIamPolicy(resource string, setiampolicyrequest *SetIamPolicyRequest) *ProjectsLocationsRegistriesSetIamPolicyCall {
	c := &ProjectsLocationsRegistriesSetIamPolicyCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.resource = resource
	c.setiampolicyrequest = setiampolicyrequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesSetIamPolicyCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesSetIamPolicyCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesSetIamPolicyCall) Context(ctx context.Context) *ProjectsLocationsRegistriesSetIamPolicyCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesSetIamPolicyCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesSetIamPolicyCall) doRequest(alt string) (*http.Response, error) {
	return nil, errors.New("Not implemented")
	// reqHeaders := make(http.Header)
	// for k, v := range c.header_ {
	// 	reqHeaders[k] = v
	// }
	// var body io.Reader = nil
	// body, err := googleapi.WithoutDataWrapper.JSONReader(c.setiampolicyrequest)
	// if err != nil {
	// 	return nil, err
	// }
	// reqHeaders.Set("Content-Type", "application/json")
	// urls := googleapi.ResolveRelative(c.s.ServiceAccountCredentials.Url, "v1/{+resource}:setIamPolicy")
	// urls += "?" + c.urlParams_.Encode()
	// req, err := http.NewRequest("POST", urls, body)
	// if err != nil {
	// 	return nil, err
	// }
	// req.Header = reqHeaders
	// googleapi.Expand(req.URL, map[string]string{
	// 	"resource": c.resource,
	// })
	// return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.setIamPolicy" call.
// Exactly one of *Policy or error will be non-nil. Any non-2xx status
// code is an error. Response headers are in either
// *Policy.ServerResponse.Header or (if a response was returned at all)
// in error.(*googleapi.Error).Header. Use googleapi.IsNotModified to
// check whether the returned error was because http.StatusNotModified
// was returned.
func (c *ProjectsLocationsRegistriesSetIamPolicyCall) Do() (*Policy, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &Policy{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Sets the access control policy on the specified resource. Replaces any existing policy.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}:setIamPolicy",
	//   "httpMethod": "POST",
	//   "id": "cloudiot.projects.locations.registries.setIamPolicy",
	//   "parameterOrder": [
	//     "resource"
	//   ],
	//   "parameters": {
	//     "resource": {
	//       "description": "REQUIRED: The resource for which the policy is being specified. See [Resource names](https://cloud.google.com/apis/design/resource_names) for the appropriate value for this field.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+resource}:setIamPolicy",
	//   "request": {
	//     "$ref": "SetIamPolicyRequest"
	//   },
	//   "response": {
	//     "$ref": "Policy"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// method id "cloudiot.projects.locations.registries.testIamPermissions":

type ProjectsLocationsRegistriesTestIamPermissionsCall struct {
	s                         *Service
	resource                  string
	testiampermissionsrequest *TestIamPermissionsRequest
	urlParams_                gensupport.URLParams
	ctx_                      context.Context
	header_                   http.Header
}

// TestIamPermissions: Returns permissions that a caller has on the
// specified resource. If the resource does not exist, this will return
// an empty set of permissions, not a NOT_FOUND error.
//
//   - resource: REQUIRED: The resource for which the policy detail is
//     being requested. See Resource names
//     (https://cloud.google.com/apis/design/resource_names) for the
//     appropriate value for this field.
func (r *ProjectsLocationsRegistriesService) TestIamPermissions(resource string, testiampermissionsrequest *TestIamPermissionsRequest) *ProjectsLocationsRegistriesTestIamPermissionsCall {
	c := &ProjectsLocationsRegistriesTestIamPermissionsCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.resource = resource
	c.testiampermissionsrequest = testiampermissionsrequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesTestIamPermissionsCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesTestIamPermissionsCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesTestIamPermissionsCall) Context(ctx context.Context) *ProjectsLocationsRegistriesTestIamPermissionsCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesTestIamPermissionsCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesTestIamPermissionsCall) doRequest(alt string) (*http.Response, error) {
	return nil, errors.New("Not implemented")
	// reqHeaders := make(http.Header)
	// for k, v := range c.header_ {
	// 	reqHeaders[k] = v
	// }
	// var body io.Reader = nil
	// body, err := googleapi.WithoutDataWrapper.JSONReader(c.testiampermissionsrequest)
	// if err != nil {
	// 	return nil, err
	// }
	// reqHeaders.Set("Content-Type", "application/json")
	// urls := googleapi.ResolveRelative(c.s.ServiceAccountCredentials.Url, "v1/{+resource}:testIamPermissions")
	// urls += "?" + c.urlParams_.Encode()
	// req, err := http.NewRequest("POST", urls, body)
	// if err != nil {
	// 	return nil, err
	// }
	// req.Header = reqHeaders
	// googleapi.Expand(req.URL, map[string]string{
	// 	"resource": c.resource,
	// })
	// return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.testIamPermissions" call.
// Exactly one of *TestIamPermissionsResponse or error will be non-nil.
// Any non-2xx status code is an error. Response headers are in either
// *TestIamPermissionsResponse.ServerResponse.Header or (if a response
// was returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ProjectsLocationsRegistriesTestIamPermissionsCall) Do() (*TestIamPermissionsResponse, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &TestIamPermissionsResponse{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Returns permissions that a caller has on the specified resource. If the resource does not exist, this will return an empty set of permissions, not a NOT_FOUND error.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}:testIamPermissions",
	//   "httpMethod": "POST",
	//   "id": "cloudiot.projects.locations.registries.testIamPermissions",
	//   "parameterOrder": [
	//     "resource"
	//   ],
	//   "parameters": {
	//     "resource": {
	//       "description": "REQUIRED: The resource for which the policy detail is being requested. See [Resource names](https://cloud.google.com/apis/design/resource_names) for the appropriate value for this field.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+resource}:testIamPermissions",
	//   "request": {
	//     "$ref": "TestIamPermissionsRequest"
	//   },
	//   "response": {
	//     "$ref": "TestIamPermissionsResponse"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// method id "cloudiot.projects.locations.registries.unbindDeviceFromGateway":

type ProjectsLocationsRegistriesUnbindDeviceFromGatewayCall struct {
	s                              *Service
	parent                         string
	unbinddevicefromgatewayrequest *UnbindDeviceFromGatewayRequest
	urlParams_                     gensupport.URLParams
	ctx_                           context.Context
	header_                        http.Header
}

// UnbindDeviceFromGateway: Deletes the association between the device
// and the gateway.
//
//   - parent: The name of the registry. For example,
//     `projects/example-project/locations/us-central1/registries/my-regist
//     ry`.
func (r *ProjectsLocationsRegistriesService) UnbindDeviceFromGateway(parent string, unbinddevicefromgatewayrequest *UnbindDeviceFromGatewayRequest) *ProjectsLocationsRegistriesUnbindDeviceFromGatewayCall {
	c := &ProjectsLocationsRegistriesUnbindDeviceFromGatewayCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.parent = parent
	c.unbinddevicefromgatewayrequest = unbinddevicefromgatewayrequest
	c.urlParams_.Set("method", "unbindDeviceFromGateway")
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesUnbindDeviceFromGatewayCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesUnbindDeviceFromGatewayCall {
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesUnbindDeviceFromGatewayCall) Context(ctx context.Context) *ProjectsLocationsRegistriesUnbindDeviceFromGatewayCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesUnbindDeviceFromGatewayCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesUnbindDeviceFromGatewayCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.unbinddevicefromgatewayrequest)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")

	matches, err := c.s.TemplatePaths.RegistryPathTemplate.Match(c.parent)
	if err != nil {
		return nil, err
	}
	registry := matches["registry"]
	location := matches["location"]
	credentials := GetRegistryCredentials(registry, location, c.s)
	reqHeaders.Set("ClearBlade-UserToken", credentials.Token)

	urls := fmt.Sprintf("%s/api/v/4/webhook/execute/%s/cloudiot", credentials.Url, credentials.SystemKey)
	urls += "?" + c.urlParams_.Encode()
	req, err := http.NewRequest("POST", urls, body)
	if err != nil {
		return nil, err
	}
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"parent": c.parent,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.unbindDeviceFromGateway" call.
// Exactly one of *UnbindDeviceFromGatewayResponse or error will be
// non-nil. Any non-2xx status code is an error. Response headers are in
// either *UnbindDeviceFromGatewayResponse.ServerResponse.Header or (if
// a response was returned at all) in error.(*googleapi.Error).Header.
// Use googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ProjectsLocationsRegistriesUnbindDeviceFromGatewayCall) Do() (*UnbindDeviceFromGatewayResponse, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &UnbindDeviceFromGatewayResponse{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Deletes the association between the device and the gateway.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}:unbindDeviceFromGateway",
	//   "httpMethod": "POST",
	//   "id": "cloudiot.projects.locations.registries.unbindDeviceFromGateway",
	//   "parameterOrder": [
	//     "parent"
	//   ],
	//   "parameters": {
	//     "parent": {
	//       "description": "Required. The name of the registry. For example, `projects/example-project/locations/us-central1/registries/my-registry`.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+parent}:unbindDeviceFromGateway",
	//   "request": {
	//     "$ref": "UnbindDeviceFromGatewayRequest"
	//   },
	//   "response": {
	//     "$ref": "UnbindDeviceFromGatewayResponse"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// method id "cloudiot.projects.locations.registries.devices.create":

type ProjectsLocationsRegistriesDevicesCreateCall struct {
	s          *Service
	parent     string
	device     *Device
	urlParams_ gensupport.URLParams
	ctx_       context.Context
	header_    http.Header
}

// Create: Creates a device in a device registry.
//
//   - parent: The name of the device registry where this device should be
//     created. For example,
//     `projects/example-project/locations/us-central1/registries/my-registry`.
func (r *ProjectsLocationsRegistriesDevicesService) Create(parent string, device *Device) *ProjectsLocationsRegistriesDevicesCreateCall {
	c := &ProjectsLocationsRegistriesDevicesCreateCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.parent = parent
	c.device = device
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesDevicesCreateCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesDevicesCreateCall {
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesDevicesCreateCall) Context(ctx context.Context) *ProjectsLocationsRegistriesDevicesCreateCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesDevicesCreateCall) Header() http.Header {
	return http.Header{}
}

func (c *ProjectsLocationsRegistriesDevicesCreateCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.device)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")
	matches, err := c.s.TemplatePaths.RegistryPathTemplate.Match(c.parent)
	if err != nil {
		return nil, err
	}
	registry := matches["registry"]
	location := matches["location"]
	credentials := GetRegistryCredentials(registry, location, c.s)
	reqHeaders.Set("ClearBlade-UserToken", credentials.Token)

	urls := fmt.Sprintf("%s/api/v/4/webhook/execute/%s/cloudiot_devices", credentials.Url, credentials.SystemKey)
	urls += "?" + c.urlParams_.Encode()
	req, err := http.NewRequest("POST", urls, body)
	if err != nil {
		return nil, err
	}
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"parent": c.parent,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.devices.create" call.
// Exactly one of *Device or error will be non-nil. Any non-2xx status
// code is an error. Response headers are in either
// *Device.ServerResponse.Header or (if a response was returned at all)
// in error.(*googleapi.Error).Header. Use googleapi.IsNotModified to
// check whether the returned error was because http.StatusNotModified
// was returned.
func (c *ProjectsLocationsRegistriesDevicesCreateCall) Do() (*Device, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &Device{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Creates a device in a device registry.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}/devices",
	//   "httpMethod": "POST",
	//   "id": "cloudiot.projects.locations.registries.devices.create",
	//   "parameterOrder": [
	//     "parent"
	//   ],
	//   "parameters": {
	//     "parent": {
	//       "description": "Required. The name of the device registry where this device should be created. For example, `projects/example-project/locations/us-central1/registries/my-registry`.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+parent}/devices",
	//   "request": {
	//     "$ref": "Device"
	//   },
	//   "response": {
	//     "$ref": "Device"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// method id "cloudiot.projects.locations.registries.devices.delete":

type ProjectsLocationsRegistriesDevicesDeleteCall struct {
	s          *Service
	name       string
	urlParams_ gensupport.URLParams
	ctx_       context.Context
	header_    http.Header
}

// Delete: Deletes a device.
//
//   - name: The name of the device. For example,
//     `projects/p0/locations/us-central1/registries/registry0/devices/devi
//     ce0` or
//     `projects/p0/locations/us-central1/registries/registry0/devices/{num
//     _id}`.
func (r *ProjectsLocationsRegistriesDevicesService) Delete(name string) *ProjectsLocationsRegistriesDevicesDeleteCall {
	c := &ProjectsLocationsRegistriesDevicesDeleteCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.name = name
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesDevicesDeleteCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesDevicesDeleteCall {
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesDevicesDeleteCall) Context(ctx context.Context) *ProjectsLocationsRegistriesDevicesDeleteCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesDevicesDeleteCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesDevicesDeleteCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	var body io.Reader = nil
	reqHeaders.Set("Content-Type", "application/json")
	matches, err := c.s.TemplatePaths.DevicePathTemplate.Match(c.name)
	if err != nil {
		return nil, err
	}
	registry := matches["registry"]
	location := matches["location"]
	credentials := GetRegistryCredentials(registry, location, c.s)
	reqHeaders.Set("ClearBlade-UserToken", credentials.Token)
	c.urlParams_.Set("name", c.name)

	urls := fmt.Sprintf("%s/api/v/4/webhook/execute/%s/cloudiot_devices", credentials.Url, credentials.SystemKey)
	urls += "?" + c.urlParams_.Encode()
	req, err := http.NewRequest("DELETE", urls, body)
	if err != nil {
		return nil, err
	}
	req.Header = reqHeaders
	// googleapi.Expand(req.URL, map[string]string{
	// 	"name": c.name,
	// })
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.devices.delete" call.
// Exactly one of *Empty or error will be non-nil. Any non-2xx status
// code is an error. Response headers are in either
// *Empty.ServerResponse.Header or (if a response was returned at all)
// in error.(*googleapi.Error).Header. Use googleapi.IsNotModified to
// check whether the returned error was because http.StatusNotModified
// was returned.
func (c *ProjectsLocationsRegistriesDevicesDeleteCall) Do() (*Empty, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &Empty{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	return ret, nil
	// {
	//   "description": "Deletes a device.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}/devices/{devicesId}",
	//   "httpMethod": "DELETE",
	//   "id": "cloudiot.projects.locations.registries.devices.delete",
	//   "parameterOrder": [
	//     "name"
	//   ],
	//   "parameters": {
	//     "name": {
	//       "description": "Required. The name of the device. For example, `projects/p0/locations/us-central1/registries/registry0/devices/device0` or `projects/p0/locations/us-central1/registries/registry0/devices/{num_id}`.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+/devices/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+name}",
	//   "response": {
	//     "$ref": "Empty"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// method id "cloudiot.projects.locations.registries.devices.get":

type ProjectsLocationsRegistriesDevicesGetCall struct {
	s            *Service
	name         string
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// Get: Gets details about a device.
//
//   - name: The name of the device. For example,
//     `projects/p0/locations/us-central1/registries/registry0/devices/devi
//     ce0` or
//     `projects/p0/locations/us-central1/registries/registry0/devices/{num
//     _id}`.
func (r *ProjectsLocationsRegistriesDevicesService) Get(name string) *ProjectsLocationsRegistriesDevicesGetCall {
	c := &ProjectsLocationsRegistriesDevicesGetCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.name = name
	return c
}

// FieldMask sets the optional parameter "fieldMask": The fields of the
// `Device` resource to be returned in the response. If the field mask
// is unset or empty, all fields are returned. Fields have to be
// provided in snake_case format, for example: `last_heartbeat_time`.
func (c *ProjectsLocationsRegistriesDevicesGetCall) FieldMask(fieldMask string) *ProjectsLocationsRegistriesDevicesGetCall {
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesDevicesGetCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesDevicesGetCall {
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *ProjectsLocationsRegistriesDevicesGetCall) IfNoneMatch(entityTag string) *ProjectsLocationsRegistriesDevicesGetCall {
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesDevicesGetCall) Context(ctx context.Context) *ProjectsLocationsRegistriesDevicesGetCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesDevicesGetCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesDevicesGetCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	reqHeaders.Set("Content-Type", "application/json")
	matches, err := c.s.TemplatePaths.DevicePathTemplate.Match(c.name)
	if err != nil {
		return nil, err
	}
	registry := matches["registry"]
	location := matches["location"]
	credentials := GetRegistryCredentials(registry, location, c.s)
	reqHeaders.Set("ClearBlade-UserToken", credentials.Token)
	c.urlParams_.Set("name", c.name)

	urls := fmt.Sprintf("%s/api/v/4/webhook/execute/%s/cloudiot_devices", credentials.Url, credentials.SystemKey)
	urls += "?" + c.urlParams_.Encode()
	req, err := http.NewRequest("GET", urls, body)
	if err != nil {
		return nil, err
	}
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"name": c.name,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.devices.get" call.
// Exactly one of *Device or error will be non-nil. Any non-2xx status
// code is an error. Response headers are in either
// *Device.ServerResponse.Header or (if a response was returned at all)
// in error.(*googleapi.Error).Header. Use googleapi.IsNotModified to
// check whether the returned error was because http.StatusNotModified
// was returned.
func (c *ProjectsLocationsRegistriesDevicesGetCall) Do() (*Device, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &Device{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Gets details about a device.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}/devices/{devicesId}",
	//   "httpMethod": "GET",
	//   "id": "cloudiot.projects.locations.registries.devices.get",
	//   "parameterOrder": [
	//     "name"
	//   ],
	//   "parameters": {
	//     "fieldMask": {
	//       "description": "The fields of the `Device` resource to be returned in the response. If the field mask is unset or empty, all fields are returned. Fields have to be provided in snake_case format, for example: `last_heartbeat_time`.",
	//       "format": "google-fieldmask",
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "name": {
	//       "description": "Required. The name of the device. For example, `projects/p0/locations/us-central1/registries/registry0/devices/device0` or `projects/p0/locations/us-central1/registries/registry0/devices/{num_id}`.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+/devices/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+name}",
	//   "response": {
	//     "$ref": "Device"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// method id "cloudiot.projects.locations.registries.devices.list":

type ProjectsLocationsRegistriesDevicesListCall struct {
	s            *Service
	parent       string
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// List: List devices in a device registry.
//
//   - parent: The device registry path. Required. For example,
//     `projects/my-project/locations/us-central1/registries/my-registry`.
func (r *ProjectsLocationsRegistriesDevicesService) List(parent string) *ProjectsLocationsRegistriesDevicesListCall {
	c := &ProjectsLocationsRegistriesDevicesListCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.parent = parent
	c.urlParams_.Set("parent", parent)
	return c
}

// DeviceIds sets the optional parameter "deviceIds": A list of device
// string IDs. For example, `['device0', 'device12']`. If empty, this
// field is ignored. Maximum IDs: 10,000
func (c *ProjectsLocationsRegistriesDevicesListCall) DeviceIds(deviceIds ...string) *ProjectsLocationsRegistriesDevicesListCall {
	c.urlParams_.SetMulti("deviceIds", append([]string{}, deviceIds...))
	return c
}

// DeviceNumIds sets the optional parameter "deviceNumIds": A list of
// device numeric IDs. If empty, this field is ignored. Maximum IDs:
// 10,000.
func (c *ProjectsLocationsRegistriesDevicesListCall) DeviceNumIds(deviceNumIds ...uint64) *ProjectsLocationsRegistriesDevicesListCall {
	var deviceNumIds_ []string
	for _, v := range deviceNumIds {
		deviceNumIds_ = append(deviceNumIds_, fmt.Sprint(v))
	}
	c.urlParams_.SetMulti("deviceNumIds", deviceNumIds_)
	return c
}

// FieldMask sets the optional parameter "fieldMask": The fields of the
// `Device` resource to be returned in the response. The fields `id` and
// `num_id` are always returned, along with any other fields specified
// in snake_case format, for example: `last_heartbeat_time`.
func (c *ProjectsLocationsRegistriesDevicesListCall) FieldMask(fieldMask string) *ProjectsLocationsRegistriesDevicesListCall {
	c.urlParams_.Set("fieldMask", fieldMask)
	return c
}

// GatewayListOptionsAssociationsDeviceId sets the optional parameter
// "gatewayListOptions.associationsDeviceId": If set, returns only the
// gateways with which the specified device is associated. The device ID
// can be numeric (`num_id`) or the user-defined string (`id`). For
// example, if `456` is specified, returns only the gateways to which
// the device with `num_id` 456 is bound.
func (c *ProjectsLocationsRegistriesDevicesListCall) GatewayListOptionsAssociationsDeviceId(gatewayListOptionsAssociationsDeviceId string) *ProjectsLocationsRegistriesDevicesListCall {
	c.urlParams_.Set("gatewayListOptions.associationsDeviceId", gatewayListOptionsAssociationsDeviceId)
	return c
}

// GatewayListOptionsAssociationsGatewayId sets the optional parameter
// "gatewayListOptions.associationsGatewayId": If set, only devices
// associated with the specified gateway are returned. The gateway ID
// can be numeric (`num_id`) or the user-defined string (`id`). For
// example, if `123` is specified, only devices bound to the gateway
// with `num_id` 123 are returned.
func (c *ProjectsLocationsRegistriesDevicesListCall) GatewayListOptionsAssociationsGatewayId(gatewayListOptionsAssociationsGatewayId string) *ProjectsLocationsRegistriesDevicesListCall {
	c.urlParams_.Set("gatewayListOptions.associationsGatewayId", gatewayListOptionsAssociationsGatewayId)
	return c
}

// GatewayListOptionsGatewayType sets the optional parameter
// "gatewayListOptions.gatewayType": If `GATEWAY` is specified, only
// gateways are returned. If `NON_GATEWAY` is specified, only
// non-gateway devices are returned. If `GATEWAY_TYPE_UNSPECIFIED` is
// specified, all devices are returned.
//
// Possible values:
//
//	"GATEWAY_TYPE_UNSPECIFIED" - If unspecified, the device is
//
// considered a non-gateway device.
//
//	"GATEWAY" - The device is a gateway.
//	"NON_GATEWAY" - The device is not a gateway.
func (c *ProjectsLocationsRegistriesDevicesListCall) GatewayListOptionsGatewayType(gatewayListOptionsGatewayType string) *ProjectsLocationsRegistriesDevicesListCall {
	c.urlParams_.Set("gatewayListOptions.gatewayType", gatewayListOptionsGatewayType)
	return c
}

// PageSize sets the optional parameter "pageSize": The maximum number
// of devices to return in the response. If this value is zero, the
// service will select a default size. A call may return fewer objects
// than requested. A non-empty `next_page_token` in the response
// indicates that more data is available.
func (c *ProjectsLocationsRegistriesDevicesListCall) PageSize(pageSize int64) *ProjectsLocationsRegistriesDevicesListCall {
	c.urlParams_.Set("pageSize", fmt.Sprint(pageSize))
	return c
}

// PageToken sets the optional parameter "pageToken": The value returned
// by the last `ListDevicesResponse`; indicates that this is a
// continuation of a prior `ListDevices` call and the system should
// return the next page of data.
func (c *ProjectsLocationsRegistriesDevicesListCall) PageToken(pageToken string) *ProjectsLocationsRegistriesDevicesListCall {
	c.urlParams_.Set("pageToken", pageToken)
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesDevicesListCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesDevicesListCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *ProjectsLocationsRegistriesDevicesListCall) IfNoneMatch(entityTag string) *ProjectsLocationsRegistriesDevicesListCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesDevicesListCall) Context(ctx context.Context) *ProjectsLocationsRegistriesDevicesListCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesDevicesListCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesDevicesListCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	matches, err := c.s.TemplatePaths.RegistryPathTemplate.Match(c.parent)
	if err != nil {
		return nil, err
	}
	registry := matches["registry"]
	location := matches["location"]
	credentials := GetRegistryCredentials(registry, location, c.s)
	reqHeaders.Set("ClearBlade-UserToken", credentials.Token)
	urls := fmt.Sprintf("%s/api/v/4/webhook/execute/%s/cloudiot_devices", credentials.Url, credentials.SystemKey)
	urls += "?" + c.urlParams_.Encode()
	req, err := http.NewRequest("GET", urls, body)
	if err != nil {
		return nil, err
	}
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"parent": c.parent,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.devices.list" call.
// Exactly one of *ListDevicesResponse or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *ListDevicesResponse.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ProjectsLocationsRegistriesDevicesListCall) Do() (*ListDevicesResponse, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &ListDevicesResponse{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "List devices in a device registry.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}/devices",
	//   "httpMethod": "GET",
	//   "id": "cloudiot.projects.locations.registries.devices.list",
	//   "parameterOrder": [
	//     "parent"
	//   ],
	//   "parameters": {
	//     "deviceIds": {
	//       "description": "A list of device string IDs. For example, `['device0', 'device12']`. If empty, this field is ignored. Maximum IDs: 10,000",
	//       "location": "query",
	//       "repeated": true,
	//       "type": "string"
	//     },
	//     "deviceNumIds": {
	//       "description": "A list of device numeric IDs. If empty, this field is ignored. Maximum IDs: 10,000.",
	//       "format": "uint64",
	//       "location": "query",
	//       "repeated": true,
	//       "type": "string"
	//     },
	//     "fieldMask": {
	//       "description": "The fields of the `Device` resource to be returned in the response. The fields `id` and `num_id` are always returned, along with any other fields specified in snake_case format, for example: `last_heartbeat_time`.",
	//       "format": "google-fieldmask",
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "gatewayListOptions.associationsDeviceId": {
	//       "description": "If set, returns only the gateways with which the specified device is associated. The device ID can be numeric (`num_id`) or the user-defined string (`id`). For example, if `456` is specified, returns only the gateways to which the device with `num_id` 456 is bound.",
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "gatewayListOptions.associationsGatewayId": {
	//       "description": "If set, only devices associated with the specified gateway are returned. The gateway ID can be numeric (`num_id`) or the user-defined string (`id`). For example, if `123` is specified, only devices bound to the gateway with `num_id` 123 are returned.",
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "gatewayListOptions.gatewayType": {
	//       "description": "If `GATEWAY` is specified, only gateways are returned. If `NON_GATEWAY` is specified, only non-gateway devices are returned. If `GATEWAY_TYPE_UNSPECIFIED` is specified, all devices are returned.",
	//       "enum": [
	//         "GATEWAY_TYPE_UNSPECIFIED",
	//         "GATEWAY",
	//         "NON_GATEWAY"
	//       ],
	//       "enumDescriptions": [
	//         "If unspecified, the device is considered a non-gateway device.",
	//         "The device is a gateway.",
	//         "The device is not a gateway."
	//       ],
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "pageSize": {
	//       "description": "The maximum number of devices to return in the response. If this value is zero, the service will select a default size. A call may return fewer objects than requested. A non-empty `next_page_token` in the response indicates that more data is available.",
	//       "format": "int32",
	//       "location": "query",
	//       "type": "integer"
	//     },
	//     "pageToken": {
	//       "description": "The value returned by the last `ListDevicesResponse`; indicates that this is a continuation of a prior `ListDevices` call and the system should return the next page of data.",
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "parent": {
	//       "description": "Required. The device registry path. Required. For example, `projects/my-project/locations/us-central1/registries/my-registry`.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+parent}/devices",
	//   "response": {
	//     "$ref": "ListDevicesResponse"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// Pages invokes f for each page of results.
// A non-nil error returned from f will halt the iteration.
// The provided context supersedes any context provided to the Context method.
func (c *ProjectsLocationsRegistriesDevicesListCall) Pages(ctx context.Context, f func(*ListDevicesResponse) error) error {
	c.ctx_ = ctx
	defer c.PageToken(c.urlParams_.Get("pageToken")) // reset paging to original point
	for {
		x, err := c.Do()
		if err != nil {
			return err
		}
		if err := f(x); err != nil {
			return err
		}
		if x.NextPageToken == "" {
			return nil
		}
		c.PageToken(x.NextPageToken)
	}
}

// method id "cloudiot.projects.locations.registries.devices.modifyCloudToDeviceConfig":

type ProjectsLocationsRegistriesDevicesModifyCloudToDeviceConfigCall struct {
	s                                *Service
	name                             string
	modifycloudtodeviceconfigrequest *ModifyCloudToDeviceConfigRequest
	urlParams_                       gensupport.URLParams
	ctx_                             context.Context
	header_                          http.Header
}

// ModifyCloudToDeviceConfig: Modifies the configuration for the device,
// which is eventually sent from the Cloud IoT Core servers. Returns the
// modified configuration version and its metadata.
//
//   - name: The name of the device. For example,
//     `projects/p0/locations/us-central1/registries/registry0/devices/devi
//     ce0` or
//     `projects/p0/locations/us-central1/registries/registry0/devices/{num
//     _id}`.
func (r *ProjectsLocationsRegistriesDevicesService) ModifyCloudToDeviceConfig(name string, modifycloudtodeviceconfigrequest *ModifyCloudToDeviceConfigRequest) *ProjectsLocationsRegistriesDevicesModifyCloudToDeviceConfigCall {
	c := &ProjectsLocationsRegistriesDevicesModifyCloudToDeviceConfigCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.name = name
	c.modifycloudtodeviceconfigrequest = modifycloudtodeviceconfigrequest
	c.urlParams_.Set("method", "modifyCloudToDeviceConfig")
	c.urlParams_.Set("name", name)
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesDevicesModifyCloudToDeviceConfigCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesDevicesModifyCloudToDeviceConfigCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesDevicesModifyCloudToDeviceConfigCall) Context(ctx context.Context) *ProjectsLocationsRegistriesDevicesModifyCloudToDeviceConfigCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesDevicesModifyCloudToDeviceConfigCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesDevicesModifyCloudToDeviceConfigCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.modifycloudtodeviceconfigrequest)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")
	matches, err := c.s.TemplatePaths.DevicePathTemplate.Match(c.name)
	if err != nil {
		return nil, err
	}
	registry := matches["registry"]
	location := matches["location"]
	credentials := GetRegistryCredentials(registry, location, c.s)
	reqHeaders.Set("ClearBlade-UserToken", credentials.Token)

	urls := fmt.Sprintf("%s/api/v/4/webhook/execute/%s/cloudiot_devices", credentials.Url, credentials.SystemKey)
	urls += "?" + c.urlParams_.Encode()
	req, err := http.NewRequest("POST", urls, body)
	if err != nil {
		return nil, err
	}
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"name": c.name,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.devices.modifyCloudToDeviceConfig" call.
// Exactly one of *DeviceConfig or error will be non-nil. Any non-2xx
// status code is an error. Response headers are in either
// *DeviceConfig.ServerResponse.Header or (if a response was returned at
// all) in error.(*googleapi.Error).Header. Use googleapi.IsNotModified
// to check whether the returned error was because
// http.StatusNotModified was returned.
func (c *ProjectsLocationsRegistriesDevicesModifyCloudToDeviceConfigCall) Do() (*DeviceConfig, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &DeviceConfig{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Modifies the configuration for the device, which is eventually sent from the Cloud IoT Core servers. Returns the modified configuration version and its metadata.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}/devices/{devicesId}:modifyCloudToDeviceConfig",
	//   "httpMethod": "POST",
	//   "id": "cloudiot.projects.locations.registries.devices.modifyCloudToDeviceConfig",
	//   "parameterOrder": [
	//     "name"
	//   ],
	//   "parameters": {
	//     "name": {
	//       "description": "Required. The name of the device. For example, `projects/p0/locations/us-central1/registries/registry0/devices/device0` or `projects/p0/locations/us-central1/registries/registry0/devices/{num_id}`.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+/devices/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+name}:modifyCloudToDeviceConfig",
	//   "request": {
	//     "$ref": "ModifyCloudToDeviceConfigRequest"
	//   },
	//   "response": {
	//     "$ref": "DeviceConfig"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// method id "cloudiot.projects.locations.registries.devices.patch":

type ProjectsLocationsRegistriesDevicesPatchCall struct {
	s          *Service
	name       string
	device     *Device
	urlParams_ gensupport.URLParams
	ctx_       context.Context
	header_    http.Header
}

// Patch: Updates a device.
//
//   - name: The resource path name. For example,
//     `projects/p1/locations/us-central1/registries/registry0/devices/dev0
//     ` or
//     `projects/p1/locations/us-central1/registries/registry0/devices/{num
//     _id}`. When `name` is populated as a response from the service, it
//     always ends in the device numeric ID.
func (r *ProjectsLocationsRegistriesDevicesService) Patch(name string, device *Device) *ProjectsLocationsRegistriesDevicesPatchCall {
	c := &ProjectsLocationsRegistriesDevicesPatchCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.name = name
	c.device = device
	return c
}

// UpdateMask sets the optional parameter "updateMask": Required. Only
// updates the `device` fields indicated by this mask. The field mask
// must not be empty, and it must not contain fields that are immutable
// or only set by the server. Mutable top-level fields: `credentials`,
// `blocked`, and `metadata`
// A comma-separated list of fully qualified names of fields. Example: "user.displayName,photo".
func (c *ProjectsLocationsRegistriesDevicesPatchCall) UpdateMask(updateMask string) *ProjectsLocationsRegistriesDevicesPatchCall {
	c.urlParams_.Set("updateMask", updateMask)
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesDevicesPatchCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesDevicesPatchCall {
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesDevicesPatchCall) Context(ctx context.Context) *ProjectsLocationsRegistriesDevicesPatchCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesDevicesPatchCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesDevicesPatchCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.device)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")
	matches, err := c.s.TemplatePaths.DevicePathTemplate.Match(c.name)
	if err != nil {
		return nil, err
	}
	registry := matches["registry"]
	location := matches["location"]
	credentials := GetRegistryCredentials(registry, location, c.s)
	reqHeaders.Set("ClearBlade-UserToken", credentials.Token)
	c.urlParams_.Set("name", c.name)

	urls := fmt.Sprintf("%s/api/v/4/webhook/execute/%s/cloudiot_devices", credentials.Url, credentials.SystemKey)
	urls += "?" + c.urlParams_.Encode()
	req, err := http.NewRequest("PATCH", urls, body)
	if err != nil {
		return nil, err
	}
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"name": c.name,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.devices.patch" call.
// Exactly one of *Device or error will be non-nil. Any non-2xx status
// code is an error. Response headers are in either
// *Device.ServerResponse.Header or (if a response was returned at all)
// in error.(*googleapi.Error).Header. Use googleapi.IsNotModified to
// check whether the returned error was because http.StatusNotModified
// was returned.
func (c *ProjectsLocationsRegistriesDevicesPatchCall) Do() (*Device, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &Device{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Updates a device.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}/devices/{devicesId}",
	//   "httpMethod": "PATCH",
	//   "id": "cloudiot.projects.locations.registries.devices.patch",
	//   "parameterOrder": [
	//     "name"
	//   ],
	//   "parameters": {
	//     "name": {
	//       "description": "The resource path name. For example, `projects/p1/locations/us-central1/registries/registry0/devices/dev0` or `projects/p1/locations/us-central1/registries/registry0/devices/{num_id}`. When `name` is populated as a response from the service, it always ends in the device numeric ID.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+/devices/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     },
	//     "updateMask": {
	//       "description": "Required. Only updates the `device` fields indicated by this mask. The field mask must not be empty, and it must not contain fields that are immutable or only set by the server. Mutable top-level fields: `credentials`, `blocked`, and `metadata`",
	//       "format": "google-fieldmask",
	//       "location": "query",
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+name}",
	//   "request": {
	//     "$ref": "Device"
	//   },
	//   "response": {
	//     "$ref": "Device"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// method id "cloudiot.projects.locations.registries.devices.sendCommandToDevice":

type ProjectsLocationsRegistriesDevicesSendCommandToDeviceCall struct {
	s                          *Service
	name                       string
	sendcommandtodevicerequest *SendCommandToDeviceRequest
	urlParams_                 gensupport.URLParams
	ctx_                       context.Context
	header_                    http.Header
}

// SendCommandToDevice: Sends a command to the specified device. In
// order for a device to be able to receive commands, it must: 1) be
// connected to Cloud IoT Core using the MQTT protocol, and 2) be
// subscribed to the group of MQTT topics specified by
// /devices/{device-id}/commands/#. This subscription will receive
// commands at the top-level topic /devices/{device-id}/commands as well
// as commands for subfolders, like
// /devices/{device-id}/commands/subfolder. Note that subscribing to
// specific subfolders is not supported. If the command could not be
// delivered to the device, this method will return an error; in
// particular, if the device is not subscribed, this method will return
// FAILED_PRECONDITION. Otherwise, this method will return OK. If the
// subscription is QoS 1, at least once delivery will be guaranteed; for
// QoS 0, no acknowledgment will be expected from the device.
//
//   - name: The name of the device. For example,
//     `projects/p0/locations/us-central1/registries/registry0/devices/devi
//     ce0` or
//     `projects/p0/locations/us-central1/registries/registry0/devices/{num
//     _id}`.
func (r *ProjectsLocationsRegistriesDevicesService) SendCommandToDevice(name string, sendcommandtodevicerequest *SendCommandToDeviceRequest) *ProjectsLocationsRegistriesDevicesSendCommandToDeviceCall {
	c := &ProjectsLocationsRegistriesDevicesSendCommandToDeviceCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.name = name
	c.sendcommandtodevicerequest = sendcommandtodevicerequest
	c.urlParams_.Set("method", "sendCommandToDevice")
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesDevicesSendCommandToDeviceCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesDevicesSendCommandToDeviceCall {
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesDevicesSendCommandToDeviceCall) Context(ctx context.Context) *ProjectsLocationsRegistriesDevicesSendCommandToDeviceCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesDevicesSendCommandToDeviceCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesDevicesSendCommandToDeviceCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.sendcommandtodevicerequest)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")
	matches, err := c.s.TemplatePaths.DevicePathTemplate.Match(c.name)
	if err != nil {
		return nil, err
	}
	registry := matches["registry"]
	location := matches["location"]
	credentials := GetRegistryCredentials(registry, location, c.s)
	reqHeaders.Set("ClearBlade-UserToken", credentials.Token)
	c.urlParams_.Set("name", c.name)
	c.urlParams_.Set("method", "sendCommandToDevice")

	urls := fmt.Sprintf("%s/api/v/4/webhook/execute/%s/cloudiot_devices", credentials.Url, credentials.SystemKey)
	urls += "?" + c.urlParams_.Encode()
	req, err := http.NewRequest("POST", urls, body)
	if err != nil {
		return nil, err
	}
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"name": c.name,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.devices.sendCommandToDevice" call.
// Exactly one of *SendCommandToDeviceResponse or error will be non-nil.
// Any non-2xx status code is an error. Response headers are in either
// *SendCommandToDeviceResponse.ServerResponse.Header or (if a response
// was returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ProjectsLocationsRegistriesDevicesSendCommandToDeviceCall) Do() (*SendCommandToDeviceResponse, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &SendCommandToDeviceResponse{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Sends a command to the specified device. In order for a device to be able to receive commands, it must: 1) be connected to Cloud IoT Core using the MQTT protocol, and 2) be subscribed to the group of MQTT topics specified by /devices/{device-id}/commands/#. This subscription will receive commands at the top-level topic /devices/{device-id}/commands as well as commands for subfolders, like /devices/{device-id}/commands/subfolder. Note that subscribing to specific subfolders is not supported. If the command could not be delivered to the device, this method will return an error; in particular, if the device is not subscribed, this method will return FAILED_PRECONDITION. Otherwise, this method will return OK. If the subscription is QoS 1, at least once delivery will be guaranteed; for QoS 0, no acknowledgment will be expected from the device.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}/devices/{devicesId}:sendCommandToDevice",
	//   "httpMethod": "POST",
	//   "id": "cloudiot.projects.locations.registries.devices.sendCommandToDevice",
	//   "parameterOrder": [
	//     "name"
	//   ],
	//   "parameters": {
	//     "name": {
	//       "description": "Required. The name of the device. For example, `projects/p0/locations/us-central1/registries/registry0/devices/device0` or `projects/p0/locations/us-central1/registries/registry0/devices/{num_id}`.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+/devices/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+name}:sendCommandToDevice",
	//   "request": {
	//     "$ref": "SendCommandToDeviceRequest"
	//   },
	//   "response": {
	//     "$ref": "SendCommandToDeviceResponse"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// method id "cloudiot.projects.locations.registries.devices.configVersions.list":

type ProjectsLocationsRegistriesDevicesConfigVersionsListCall struct {
	s            *Service
	name         string
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// List: Lists the last few versions of the device configuration in
// descending order (i.e.: newest first).
//
//   - name: The name of the device. For example,
//     `projects/p0/locations/us-central1/registries/registry0/devices/devi
//     ce0` or
//     `projects/p0/locations/us-central1/registries/registry0/devices/{num
//     _id}`.
func (r *ProjectsLocationsRegistriesDevicesConfigVersionsService) List(name string) *ProjectsLocationsRegistriesDevicesConfigVersionsListCall {
	c := &ProjectsLocationsRegistriesDevicesConfigVersionsListCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.name = name
	c.urlParams_.Set("name", name)
	return c
}

// NumVersions sets the optional parameter "numVersions": The number of
// versions to list. Versions are listed in decreasing order of the
// version number. The maximum number of versions retained is 10. If
// this value is zero, it will return all the versions available.
func (c *ProjectsLocationsRegistriesDevicesConfigVersionsListCall) NumVersions(numVersions int64) *ProjectsLocationsRegistriesDevicesConfigVersionsListCall {
	c.urlParams_.Set("numVersions", fmt.Sprint(numVersions))
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesDevicesConfigVersionsListCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesDevicesConfigVersionsListCall {
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *ProjectsLocationsRegistriesDevicesConfigVersionsListCall) IfNoneMatch(entityTag string) *ProjectsLocationsRegistriesDevicesConfigVersionsListCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesDevicesConfigVersionsListCall) Context(ctx context.Context) *ProjectsLocationsRegistriesDevicesConfigVersionsListCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesDevicesConfigVersionsListCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesDevicesConfigVersionsListCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	matches, err := c.s.TemplatePaths.DevicePathTemplate.Match(c.name)
	if err != nil {
		return nil, err
	}
	registry := matches["registry"]
	location := matches["location"]
	credentials := GetRegistryCredentials(registry, location, c.s)
	reqHeaders.Set("ClearBlade-UserToken", credentials.Token)

	urls := fmt.Sprintf("%s/api/v/4/webhook/execute/%s/cloudiot_devices_configVersions", credentials.Url, credentials.SystemKey)
	urls += "?" + c.urlParams_.Encode()
	req, err := http.NewRequest("GET", urls, body)
	if err != nil {
		return nil, err
	}
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"name": c.name,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.devices.configVersions.list" call.
// Exactly one of *ListDeviceConfigVersionsResponse or error will be
// non-nil. Any non-2xx status code is an error. Response headers are in
// either *ListDeviceConfigVersionsResponse.ServerResponse.Header or (if
// a response was returned at all) in error.(*googleapi.Error).Header.
// Use googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ProjectsLocationsRegistriesDevicesConfigVersionsListCall) Do() (*ListDeviceConfigVersionsResponse, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &ListDeviceConfigVersionsResponse{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Lists the last few versions of the device configuration in descending order (i.e.: newest first).",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}/devices/{devicesId}/configVersions",
	//   "httpMethod": "GET",
	//   "id": "cloudiot.projects.locations.registries.devices.configVersions.list",
	//   "parameterOrder": [
	//     "name"
	//   ],
	//   "parameters": {
	//     "name": {
	//       "description": "Required. The name of the device. For example, `projects/p0/locations/us-central1/registries/registry0/devices/device0` or `projects/p0/locations/us-central1/registries/registry0/devices/{num_id}`.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+/devices/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     },
	//     "numVersions": {
	//       "description": "The number of versions to list. Versions are listed in decreasing order of the version number. The maximum number of versions retained is 10. If this value is zero, it will return all the versions available.",
	//       "format": "int32",
	//       "location": "query",
	//       "type": "integer"
	//     }
	//   },
	//   "path": "v1/{+name}/configVersions",
	//   "response": {
	//     "$ref": "ListDeviceConfigVersionsResponse"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// method id "cloudiot.projects.locations.registries.devices.states.list":

type ProjectsLocationsRegistriesDevicesStatesListCall struct {
	s            *Service
	name         string
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// List: Lists the last few versions of the device state in descending
// order (i.e.: newest first).
//
//   - name: The name of the device. For example,
//     `projects/p0/locations/us-central1/registries/registry0/devices/devi
//     ce0` or
//     `projects/p0/locations/us-central1/registries/registry0/devices/{num
//     _id}`.
func (r *ProjectsLocationsRegistriesDevicesStatesService) List(name string) *ProjectsLocationsRegistriesDevicesStatesListCall {
	c := &ProjectsLocationsRegistriesDevicesStatesListCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.name = name
	c.urlParams_.Set("name", name)
	return c
}

// NumStates sets the optional parameter "numStates": The number of
// states to list. States are listed in descending order of update time.
// The maximum number of states retained is 10. If this value is zero,
// it will return all the states available.
func (c *ProjectsLocationsRegistriesDevicesStatesListCall) NumStates(numStates int64) *ProjectsLocationsRegistriesDevicesStatesListCall {
	c.urlParams_.Set("numStates", fmt.Sprint(numStates))
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesDevicesStatesListCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesDevicesStatesListCall {
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *ProjectsLocationsRegistriesDevicesStatesListCall) IfNoneMatch(entityTag string) *ProjectsLocationsRegistriesDevicesStatesListCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesDevicesStatesListCall) Context(ctx context.Context) *ProjectsLocationsRegistriesDevicesStatesListCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesDevicesStatesListCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesDevicesStatesListCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	matches, err := c.s.TemplatePaths.DevicePathTemplate.Match(c.name)
	if err != nil {
		return nil, err
	}
	registry := matches["registry"]
	location := matches["location"]
	credentials := GetRegistryCredentials(registry, location, c.s)
	reqHeaders.Set("ClearBlade-UserToken", credentials.Token)

	urls := fmt.Sprintf("%s/api/v/4/webhook/execute/%s/cloudiot_devices_states", credentials.Url, credentials.SystemKey)
	urls += "?" + c.urlParams_.Encode()
	req, err := http.NewRequest("GET", urls, body)
	if err != nil {
		return nil, err
	}
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"name": c.name,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.devices.states.list" call.
// Exactly one of *ListDeviceStatesResponse or error will be non-nil.
// Any non-2xx status code is an error. Response headers are in either
// *ListDeviceStatesResponse.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ProjectsLocationsRegistriesDevicesStatesListCall) Do() (*ListDeviceStatesResponse, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &ListDeviceStatesResponse{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Lists the last few versions of the device state in descending order (i.e.: newest first).",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}/devices/{devicesId}/states",
	//   "httpMethod": "GET",
	//   "id": "cloudiot.projects.locations.registries.devices.states.list",
	//   "parameterOrder": [
	//     "name"
	//   ],
	//   "parameters": {
	//     "name": {
	//       "description": "Required. The name of the device. For example, `projects/p0/locations/us-central1/registries/registry0/devices/device0` or `projects/p0/locations/us-central1/registries/registry0/devices/{num_id}`.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+/devices/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     },
	//     "numStates": {
	//       "description": "The number of states to list. States are listed in descending order of update time. The maximum number of states retained is 10. If this value is zero, it will return all the states available.",
	//       "format": "int32",
	//       "location": "query",
	//       "type": "integer"
	//     }
	//   },
	//   "path": "v1/{+name}/states",
	//   "response": {
	//     "$ref": "ListDeviceStatesResponse"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// method id "cloudiot.projects.locations.registries.groups.getIamPolicy":

type ProjectsLocationsRegistriesGroupsGetIamPolicyCall struct {
	s                   *Service
	resource            string
	getiampolicyrequest *GetIamPolicyRequest
	urlParams_          gensupport.URLParams
	ctx_                context.Context
	header_             http.Header
}

// GetIamPolicy: Gets the access control policy for a resource. Returns
// an empty policy if the resource exists and does not have a policy
// set.
//
//   - resource: REQUIRED: The resource for which the policy is being
//     requested. See Resource names
//     (https://cloud.google.com/apis/design/resource_names) for the
//     appropriate value for this field.
func (r *ProjectsLocationsRegistriesGroupsService) GetIamPolicy(resource string, getiampolicyrequest *GetIamPolicyRequest) *ProjectsLocationsRegistriesGroupsGetIamPolicyCall {
	c := &ProjectsLocationsRegistriesGroupsGetIamPolicyCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.resource = resource
	c.getiampolicyrequest = getiampolicyrequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesGroupsGetIamPolicyCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesGroupsGetIamPolicyCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesGroupsGetIamPolicyCall) Context(ctx context.Context) *ProjectsLocationsRegistriesGroupsGetIamPolicyCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesGroupsGetIamPolicyCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesGroupsGetIamPolicyCall) doRequest(alt string) (*http.Response, error) {
	return nil, errors.New("Not implemented")
	// reqHeaders := make(http.Header)
	// for k, v := range c.header_ {
	// 	reqHeaders[k] = v
	// }
	// var body io.Reader = nil
	// body, err := googleapi.WithoutDataWrapper.JSONReader(c.getiampolicyrequest)
	// if err != nil {
	// 	return nil, err
	// }
	// reqHeaders.Set("Content-Type", "application/json")
	// urls := googleapi.ResolveRelative(c.s.ServiceAccountCredentials.Url, "v1/{+resource}:getIamPolicy")
	// urls += "?" + c.urlParams_.Encode()
	// req, err := http.NewRequest("POST", urls, body)
	// if err != nil {
	// 	return nil, err
	// }
	// req.Header = reqHeaders
	// googleapi.Expand(req.URL, map[string]string{
	// 	"resource": c.resource,
	// })
	// return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.groups.getIamPolicy" call.
// Exactly one of *Policy or error will be non-nil. Any non-2xx status
// code is an error. Response headers are in either
// *Policy.ServerResponse.Header or (if a response was returned at all)
// in error.(*googleapi.Error).Header. Use googleapi.IsNotModified to
// check whether the returned error was because http.StatusNotModified
// was returned.
func (c *ProjectsLocationsRegistriesGroupsGetIamPolicyCall) Do() (*Policy, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &Policy{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Gets the access control policy for a resource. Returns an empty policy if the resource exists and does not have a policy set.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}/groups/{groupsId}:getIamPolicy",
	//   "httpMethod": "POST",
	//   "id": "cloudiot.projects.locations.registries.groups.getIamPolicy",
	//   "parameterOrder": [
	//     "resource"
	//   ],
	//   "parameters": {
	//     "resource": {
	//       "description": "REQUIRED: The resource for which the policy is being requested. See [Resource names](https://cloud.google.com/apis/design/resource_names) for the appropriate value for this field.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+/groups/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+resource}:getIamPolicy",
	//   "request": {
	//     "$ref": "GetIamPolicyRequest"
	//   },
	//   "response": {
	//     "$ref": "Policy"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// method id "cloudiot.projects.locations.registries.groups.setIamPolicy":

type ProjectsLocationsRegistriesGroupsSetIamPolicyCall struct {
	s                   *Service
	resource            string
	setiampolicyrequest *SetIamPolicyRequest
	urlParams_          gensupport.URLParams
	ctx_                context.Context
	header_             http.Header
}

// SetIamPolicy: Sets the access control policy on the specified
// resource. Replaces any existing policy.
//
//   - resource: REQUIRED: The resource for which the policy is being
//     specified. See Resource names
//     (https://cloud.google.com/apis/design/resource_names) for the
//     appropriate value for this field.
func (r *ProjectsLocationsRegistriesGroupsService) SetIamPolicy(resource string, setiampolicyrequest *SetIamPolicyRequest) *ProjectsLocationsRegistriesGroupsSetIamPolicyCall {
	c := &ProjectsLocationsRegistriesGroupsSetIamPolicyCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.resource = resource
	c.setiampolicyrequest = setiampolicyrequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesGroupsSetIamPolicyCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesGroupsSetIamPolicyCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesGroupsSetIamPolicyCall) Context(ctx context.Context) *ProjectsLocationsRegistriesGroupsSetIamPolicyCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesGroupsSetIamPolicyCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesGroupsSetIamPolicyCall) doRequest(alt string) (*http.Response, error) {
	return nil, errors.New("Not implemented")
	// reqHeaders := make(http.Header)
	// for k, v := range c.header_ {
	// 	reqHeaders[k] = v
	// }
	// var body io.Reader = nil
	// body, err := googleapi.WithoutDataWrapper.JSONReader(c.setiampolicyrequest)
	// if err != nil {
	// 	return nil, err
	// }
	// reqHeaders.Set("Content-Type", "application/json")
	// urls := googleapi.ResolveRelative(c.s.ServiceAccountCredentials.Url, "v1/{+resource}:setIamPolicy")
	// urls += "?" + c.urlParams_.Encode()
	// req, err := http.NewRequest("POST", urls, body)
	// if err != nil {
	// 	return nil, err
	// }
	// req.Header = reqHeaders
	// googleapi.Expand(req.URL, map[string]string{
	// 	"resource": c.resource,
	// })
	// return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.groups.setIamPolicy" call.
// Exactly one of *Policy or error will be non-nil. Any non-2xx status
// code is an error. Response headers are in either
// *Policy.ServerResponse.Header or (if a response was returned at all)
// in error.(*googleapi.Error).Header. Use googleapi.IsNotModified to
// check whether the returned error was because http.StatusNotModified
// was returned.
func (c *ProjectsLocationsRegistriesGroupsSetIamPolicyCall) Do() (*Policy, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &Policy{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Sets the access control policy on the specified resource. Replaces any existing policy.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}/groups/{groupsId}:setIamPolicy",
	//   "httpMethod": "POST",
	//   "id": "cloudiot.projects.locations.registries.groups.setIamPolicy",
	//   "parameterOrder": [
	//     "resource"
	//   ],
	//   "parameters": {
	//     "resource": {
	//       "description": "REQUIRED: The resource for which the policy is being specified. See [Resource names](https://cloud.google.com/apis/design/resource_names) for the appropriate value for this field.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+/groups/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+resource}:setIamPolicy",
	//   "request": {
	//     "$ref": "SetIamPolicyRequest"
	//   },
	//   "response": {
	//     "$ref": "Policy"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// method id "cloudiot.projects.locations.registries.groups.testIamPermissions":

type ProjectsLocationsRegistriesGroupsTestIamPermissionsCall struct {
	s                         *Service
	resource                  string
	testiampermissionsrequest *TestIamPermissionsRequest
	urlParams_                gensupport.URLParams
	ctx_                      context.Context
	header_                   http.Header
}

// TestIamPermissions: Returns permissions that a caller has on the
// specified resource. If the resource does not exist, this will return
// an empty set of permissions, not a NOT_FOUND error.
//
//   - resource: REQUIRED: The resource for which the policy detail is
//     being requested. See Resource names
//     (https://cloud.google.com/apis/design/resource_names) for the
//     appropriate value for this field.
func (r *ProjectsLocationsRegistriesGroupsService) TestIamPermissions(resource string, testiampermissionsrequest *TestIamPermissionsRequest) *ProjectsLocationsRegistriesGroupsTestIamPermissionsCall {
	c := &ProjectsLocationsRegistriesGroupsTestIamPermissionsCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.resource = resource
	c.testiampermissionsrequest = testiampermissionsrequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesGroupsTestIamPermissionsCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesGroupsTestIamPermissionsCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesGroupsTestIamPermissionsCall) Context(ctx context.Context) *ProjectsLocationsRegistriesGroupsTestIamPermissionsCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesGroupsTestIamPermissionsCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesGroupsTestIamPermissionsCall) doRequest(alt string) (*http.Response, error) {
	return nil, errors.New("Not implemented")
	// reqHeaders := make(http.Header)
	// for k, v := range c.header_ {
	// 	reqHeaders[k] = v
	// }
	// var body io.Reader = nil
	// body, err := googleapi.WithoutDataWrapper.JSONReader(c.testiampermissionsrequest)
	// if err != nil {
	// 	return nil, err
	// }
	// reqHeaders.Set("Content-Type", "application/json")
	// urls := googleapi.ResolveRelative(c.s.ServiceAccountCredentials.Url, "v1/{+resource}:testIamPermissions")
	// urls += "?" + c.urlParams_.Encode()
	// req, err := http.NewRequest("POST", urls, body)
	// if err != nil {
	// 	return nil, err
	// }
	// req.Header = reqHeaders
	// googleapi.Expand(req.URL, map[string]string{
	// 	"resource": c.resource,
	// })
	// return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.groups.testIamPermissions" call.
// Exactly one of *TestIamPermissionsResponse or error will be non-nil.
// Any non-2xx status code is an error. Response headers are in either
// *TestIamPermissionsResponse.ServerResponse.Header or (if a response
// was returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ProjectsLocationsRegistriesGroupsTestIamPermissionsCall) Do() (*TestIamPermissionsResponse, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &TestIamPermissionsResponse{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Returns permissions that a caller has on the specified resource. If the resource does not exist, this will return an empty set of permissions, not a NOT_FOUND error.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}/groups/{groupsId}:testIamPermissions",
	//   "httpMethod": "POST",
	//   "id": "cloudiot.projects.locations.registries.groups.testIamPermissions",
	//   "parameterOrder": [
	//     "resource"
	//   ],
	//   "parameters": {
	//     "resource": {
	//       "description": "REQUIRED: The resource for which the policy detail is being requested. See [Resource names](https://cloud.google.com/apis/design/resource_names) for the appropriate value for this field.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+/groups/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+resource}:testIamPermissions",
	//   "request": {
	//     "$ref": "TestIamPermissionsRequest"
	//   },
	//   "response": {
	//     "$ref": "TestIamPermissionsResponse"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// method id "cloudiot.projects.locations.registries.groups.devices.list":

type ProjectsLocationsRegistriesGroupsDevicesListCall struct {
	s            *Service
	parent       string
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// List: List devices in a device registry.
//
//   - parent: The device registry path. Required. For example,
//     `projects/my-project/locations/us-central1/registries/my-registry`.
func (r *ProjectsLocationsRegistriesGroupsDevicesService) List(parent string) *ProjectsLocationsRegistriesGroupsDevicesListCall {
	c := &ProjectsLocationsRegistriesGroupsDevicesListCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.parent = parent
	return c
}

// DeviceIds sets the optional parameter "deviceIds": A list of device
// string IDs. For example, `['device0', 'device12']`. If empty, this
// field is ignored. Maximum IDs: 10,000
func (c *ProjectsLocationsRegistriesGroupsDevicesListCall) DeviceIds(deviceIds ...string) *ProjectsLocationsRegistriesGroupsDevicesListCall {
	c.urlParams_.SetMulti("deviceIds", append([]string{}, deviceIds...))
	return c
}

// DeviceNumIds sets the optional parameter "deviceNumIds": A list of
// device numeric IDs. If empty, this field is ignored. Maximum IDs:
// 10,000.
func (c *ProjectsLocationsRegistriesGroupsDevicesListCall) DeviceNumIds(deviceNumIds ...uint64) *ProjectsLocationsRegistriesGroupsDevicesListCall {
	var deviceNumIds_ []string
	for _, v := range deviceNumIds {
		deviceNumIds_ = append(deviceNumIds_, fmt.Sprint(v))
	}
	c.urlParams_.SetMulti("deviceNumIds", deviceNumIds_)
	return c
}

// FieldMask sets the optional parameter "fieldMask": The fields of the
// `Device` resource to be returned in the response. The fields `id` and
// `num_id` are always returned, along with any other fields specified
// in snake_case format, for example: `last_heartbeat_time`.
func (c *ProjectsLocationsRegistriesGroupsDevicesListCall) FieldMask(fieldMask string) *ProjectsLocationsRegistriesGroupsDevicesListCall {
	c.urlParams_.Set("fieldMask", fieldMask)
	return c
}

// GatewayListOptionsAssociationsDeviceId sets the optional parameter
// "gatewayListOptions.associationsDeviceId": If set, returns only the
// gateways with which the specified device is associated. The device ID
// can be numeric (`num_id`) or the user-defined string (`id`). For
// example, if `456` is specified, returns only the gateways to which
// the device with `num_id` 456 is bound.
func (c *ProjectsLocationsRegistriesGroupsDevicesListCall) GatewayListOptionsAssociationsDeviceId(gatewayListOptionsAssociationsDeviceId string) *ProjectsLocationsRegistriesGroupsDevicesListCall {
	c.urlParams_.Set("gatewayListOptions.associationsDeviceId", gatewayListOptionsAssociationsDeviceId)
	return c
}

// GatewayListOptionsAssociationsGatewayId sets the optional parameter
// "gatewayListOptions.associationsGatewayId": If set, only devices
// associated with the specified gateway are returned. The gateway ID
// can be numeric (`num_id`) or the user-defined string (`id`). For
// example, if `123` is specified, only devices bound to the gateway
// with `num_id` 123 are returned.
func (c *ProjectsLocationsRegistriesGroupsDevicesListCall) GatewayListOptionsAssociationsGatewayId(gatewayListOptionsAssociationsGatewayId string) *ProjectsLocationsRegistriesGroupsDevicesListCall {
	c.urlParams_.Set("gatewayListOptions.associationsGatewayId", gatewayListOptionsAssociationsGatewayId)
	return c
}

// GatewayListOptionsGatewayType sets the optional parameter
// "gatewayListOptions.gatewayType": If `GATEWAY` is specified, only
// gateways are returned. If `NON_GATEWAY` is specified, only
// non-gateway devices are returned. If `GATEWAY_TYPE_UNSPECIFIED` is
// specified, all devices are returned.
//
// Possible values:
//
//	"GATEWAY_TYPE_UNSPECIFIED" - If unspecified, the device is
//
// considered a non-gateway device.
//
//	"GATEWAY" - The device is a gateway.
//	"NON_GATEWAY" - The device is not a gateway.
func (c *ProjectsLocationsRegistriesGroupsDevicesListCall) GatewayListOptionsGatewayType(gatewayListOptionsGatewayType string) *ProjectsLocationsRegistriesGroupsDevicesListCall {
	c.urlParams_.Set("gatewayListOptions.gatewayType", gatewayListOptionsGatewayType)
	return c
}

// PageSize sets the optional parameter "pageSize": The maximum number
// of devices to return in the response. If this value is zero, the
// service will select a default size. A call may return fewer objects
// than requested. A non-empty `next_page_token` in the response
// indicates that more data is available.
func (c *ProjectsLocationsRegistriesGroupsDevicesListCall) PageSize(pageSize int64) *ProjectsLocationsRegistriesGroupsDevicesListCall {
	c.urlParams_.Set("pageSize", fmt.Sprint(pageSize))
	return c
}

// PageToken sets the optional parameter "pageToken": The value returned
// by the last `ListDevicesResponse`; indicates that this is a
// continuation of a prior `ListDevices` call and the system should
// return the next page of data.
func (c *ProjectsLocationsRegistriesGroupsDevicesListCall) PageToken(pageToken string) *ProjectsLocationsRegistriesGroupsDevicesListCall {
	c.urlParams_.Set("pageToken", pageToken)
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ProjectsLocationsRegistriesGroupsDevicesListCall) Fields(s ...googleapi.Field) *ProjectsLocationsRegistriesGroupsDevicesListCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *ProjectsLocationsRegistriesGroupsDevicesListCall) IfNoneMatch(entityTag string) *ProjectsLocationsRegistriesGroupsDevicesListCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ProjectsLocationsRegistriesGroupsDevicesListCall) Context(ctx context.Context) *ProjectsLocationsRegistriesGroupsDevicesListCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ProjectsLocationsRegistriesGroupsDevicesListCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ProjectsLocationsRegistriesGroupsDevicesListCall) doRequest(alt string) (*http.Response, error) {
	return nil, errors.New("Not implemented")
	// reqHeaders := make(http.Header)
	// for k, v := range c.header_ {
	// 	reqHeaders[k] = v
	// }
	// if c.ifNoneMatch_ != "" {
	// 	reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	// }
	// var body io.Reader = nil
	// urls := googleapi.ResolveRelative(c.s.ServiceAccountCredentials.Url, "v1/{+parent}/devices")
	// urls += "?" + c.urlParams_.Encode()
	// req, err := http.NewRequest("GET", urls, body)
	// if err != nil {
	// 	return nil, err
	// }
	// req.Header = reqHeaders
	// googleapi.Expand(req.URL, map[string]string{
	// 	"parent": c.parent,
	// })
	// return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "cloudiot.projects.locations.registries.groups.devices.list" call.
// Exactly one of *ListDevicesResponse or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *ListDevicesResponse.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ProjectsLocationsRegistriesGroupsDevicesListCall) Do() (*ListDevicesResponse, error) {
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, gensupport.WrapError(&googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		})
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 || res.StatusCode < 200 {
		return nil, createHTTPError(res)
	}
	ret := &ListDevicesResponse{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "List devices in a device registry.",
	//   "flatPath": "v1/projects/{projectsId}/locations/{locationsId}/registries/{registriesId}/groups/{groupsId}/devices",
	//   "httpMethod": "GET",
	//   "id": "cloudiot.projects.locations.registries.groups.devices.list",
	//   "parameterOrder": [
	//     "parent"
	//   ],
	//   "parameters": {
	//     "deviceIds": {
	//       "description": "A list of device string IDs. For example, `['device0', 'device12']`. If empty, this field is ignored. Maximum IDs: 10,000",
	//       "location": "query",
	//       "repeated": true,
	//       "type": "string"
	//     },
	//     "deviceNumIds": {
	//       "description": "A list of device numeric IDs. If empty, this field is ignored. Maximum IDs: 10,000.",
	//       "format": "uint64",
	//       "location": "query",
	//       "repeated": true,
	//       "type": "string"
	//     },
	//     "fieldMask": {
	//       "description": "The fields of the `Device` resource to be returned in the response. The fields `id` and `num_id` are always returned, along with any other fields specified in snake_case format, for example: `last_heartbeat_time`.",
	//       "format": "google-fieldmask",
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "gatewayListOptions.associationsDeviceId": {
	//       "description": "If set, returns only the gateways with which the specified device is associated. The device ID can be numeric (`num_id`) or the user-defined string (`id`). For example, if `456` is specified, returns only the gateways to which the device with `num_id` 456 is bound.",
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "gatewayListOptions.associationsGatewayId": {
	//       "description": "If set, only devices associated with the specified gateway are returned. The gateway ID can be numeric (`num_id`) or the user-defined string (`id`). For example, if `123` is specified, only devices bound to the gateway with `num_id` 123 are returned.",
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "gatewayListOptions.gatewayType": {
	//       "description": "If `GATEWAY` is specified, only gateways are returned. If `NON_GATEWAY` is specified, only non-gateway devices are returned. If `GATEWAY_TYPE_UNSPECIFIED` is specified, all devices are returned.",
	//       "enum": [
	//         "GATEWAY_TYPE_UNSPECIFIED",
	//         "GATEWAY",
	//         "NON_GATEWAY"
	//       ],
	//       "enumDescriptions": [
	//         "If unspecified, the device is considered a non-gateway device.",
	//         "The device is a gateway.",
	//         "The device is not a gateway."
	//       ],
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "pageSize": {
	//       "description": "The maximum number of devices to return in the response. If this value is zero, the service will select a default size. A call may return fewer objects than requested. A non-empty `next_page_token` in the response indicates that more data is available.",
	//       "format": "int32",
	//       "location": "query",
	//       "type": "integer"
	//     },
	//     "pageToken": {
	//       "description": "The value returned by the last `ListDevicesResponse`; indicates that this is a continuation of a prior `ListDevices` call and the system should return the next page of data.",
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "parent": {
	//       "description": "Required. The device registry path. Required. For example, `projects/my-project/locations/us-central1/registries/my-registry`.",
	//       "location": "path",
	//       "pattern": "^projects/[^/]+/locations/[^/]+/registries/[^/]+/groups/[^/]+$",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/{+parent}/devices",
	//   "response": {
	//     "$ref": "ListDevicesResponse"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/cloud-platform",
	//     "https://www.googleapis.com/auth/cloudiot"
	//   ]
	// }

}

// Pages invokes f for each page of results.
// A non-nil error returned from f will halt the iteration.
// The provided context supersedes any context provided to the Context method.
func (c *ProjectsLocationsRegistriesGroupsDevicesListCall) Pages(ctx context.Context, f func(*ListDevicesResponse) error) error {
	c.ctx_ = ctx
	defer c.PageToken(c.urlParams_.Get("pageToken")) // reset paging to original point
	for {
		x, err := c.Do()
		if err != nil {
			return err
		}
		if err := f(x); err != nil {
			return err
		}
		if x.NextPageToken == "" {
			return nil
		}
		c.PageToken(x.NextPageToken)
	}
}
