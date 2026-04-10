package installmanager

import (
	"context"
	"testing"

	"github.com/IBM/platform-services-go-sdk/iamidentityv1"
	"go.uber.org/mock/gomock"
	"github.com/openshift/hive/pkg/ibmclient"
	mockibm "github.com/openshift/hive/pkg/ibmclient/mock"
	"github.com/stretchr/testify/assert"

	"k8s.io/utils/ptr"
)

func TestGetCISInstanceCRN(t *testing.T) {
	tests := []struct {
		name                   string
		baseDomain             string
		existingZones          []ibmclient.DNSZoneResponse
		expectedCISInstanceCRN string
		expectErr              bool
	}{
		{
			name:       "BaseDomain Zone Found",
			baseDomain: "hive.test.example.com",
			existingZones: []ibmclient.DNSZoneResponse{
				{
					ID:             "435435",
					Name:           "hive.test.example.com",
					CISInstanceCRN: "crn:v1:bluemix:public:internet-svcs:global:a/fakehivetestexamplecom::",
				},
				{
					ID:             "343242",
					Name:           "wrong.test.example.com",
					CISInstanceCRN: "crn:v1:bluemix:public:internet-svcs:global:a/fakewrongtestexamplecom::",
				},
			},
			expectedCISInstanceCRN: "crn:v1:bluemix:public:internet-svcs:global:a/fakehivetestexamplecom::",
			expectErr:              false,
		},
		{
			name:       "BaseDomain Zone Not Found",
			baseDomain: "hive.test.example.com",
			existingZones: []ibmclient.DNSZoneResponse{
				{
					ID:             "435435",
					Name:           "wrong.hive.test.example.com",
					CISInstanceCRN: "crn:v1:bluemix:public:internet-svcs:global:a/fakewronghivetestexamplecom::",
				},
				{
					ID:             "343242",
					Name:           "alsowrong.hive.test.example.com",
					CISInstanceCRN: "crn:v1:bluemix:public:internet-svcs:global:a/alsowronghivetestexamplecom::",
				},
			},
			expectErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)

			ibmcloudClient := mockibm.NewMockAPI(mockCtrl)

			if test.existingZones != nil {
				ibmcloudClient.EXPECT().GetDNSZones(gomock.Any()).Return(test.existingZones, nil).Times(1)
			}

			cisInstanceCRN, err := ibmclient.GetCISInstanceCRN(ibmcloudClient, context.TODO(), test.baseDomain)
			if test.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if test.expectedCISInstanceCRN != "" {
				assert.Equal(t, test.expectedCISInstanceCRN, cisInstanceCRN, "found unexpected CISInstanceCRN")
			}
		})
	}
}

func TestGetAccountID(t *testing.T) {
	tests := []struct {
		name              string
		existingAPIKey    *iamidentityv1.APIKey
		expectedAccountID string
		expectErr         bool
	}{
		{
			name: "AccountID Found",
			existingAPIKey: &iamidentityv1.APIKey{
				AccountID: ptr.To("testaccountid"),
			},
			expectErr: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)

			ibmcloudClient := mockibm.NewMockAPI(mockCtrl)

			if test.existingAPIKey != nil {
				ibmcloudClient.EXPECT().GetAuthenticatorAPIKeyDetails(gomock.Any()).Return(test.existingAPIKey, nil).Times(1)
			}

			accountID, err := ibmclient.GetAccountID(ibmcloudClient, context.TODO())
			if test.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if test.expectedAccountID != "" {
				assert.Equal(t, test.expectedAccountID, accountID, "found unexpected AccountID")
			}
		})
	}
}
