// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package addon contains the service to manage addons.
package addon

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/amazon-ecs-cli-v2/internal/pkg/template"
	"github.com/aws/amazon-ecs-cli-v2/internal/pkg/template/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestDynamoDB_MarshalBinary(t *testing.T) {
	testCases := map[string]struct {
		mockDependencies func(ctrl *gomock.Controller, ddb *DynamoDB)

		wantedBinary []byte
		wantedError  error
	}{
		"error parsing template": {
			mockDependencies: func(ctrl *gomock.Controller, ddb *DynamoDB) {
				m := mocks.NewMockParser(ctrl)
				ddb.parser = m
				m.EXPECT().Parse(dynamoDbAddonPath, *ddb, gomock.Any()).Return(nil, errors.New("some error"))
			},

			wantedError: errors.New("some error"),
		},
		"returns rendered content": {
			mockDependencies: func(ctrl *gomock.Controller, ddb *DynamoDB) {
				m := mocks.NewMockParser(ctrl)
				ddb.parser = m
				m.EXPECT().Parse(dynamoDbAddonPath, *ddb, gomock.Any()).Return(&template.Content{Buffer: bytes.NewBufferString("hello")}, nil)

			},

			wantedBinary: []byte("hello"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			addon := &DynamoDB{}
			tc.mockDependencies(ctrl, addon)

			// WHEN
			b, err := addon.MarshalBinary()

			// THEN
			require.Equal(t, tc.wantedError, err)
			require.Equal(t, tc.wantedBinary, b)
		})
	}
}

func TestS3_MarshalBinary(t *testing.T) {
	testCases := map[string]struct {
		mockDependencies func(ctrl *gomock.Controller, s3 *S3)

		wantedBinary []byte
		wantedError  error
	}{
		"error parsing template": {
			mockDependencies: func(ctrl *gomock.Controller, s3 *S3) {
				m := mocks.NewMockParser(ctrl)
				s3.parser = m
				m.EXPECT().Parse(s3AddonPath, *s3, gomock.Any()).Return(nil, errors.New("some error"))
			},

			wantedError: errors.New("some error"),
		},
		"returns rendered content": {
			mockDependencies: func(ctrl *gomock.Controller, s3 *S3) {
				m := mocks.NewMockParser(ctrl)
				s3.parser = m
				m.EXPECT().Parse(s3AddonPath, *s3, gomock.Any()).Return(&template.Content{Buffer: bytes.NewBufferString("hello")}, nil)

			},

			wantedBinary: []byte("hello"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			addon := &S3{}
			tc.mockDependencies(ctrl, addon)

			// WHEN
			b, err := addon.MarshalBinary()

			// THEN
			require.Equal(t, tc.wantedError, err)
			require.Equal(t, tc.wantedBinary, b)
		})
	}
}

func TestDDBAttributeFromKey(t *testing.T) {
	testCases := map[string]struct {
		input     string
		wantName  string
		wantType  string
		wantError error
	}{
		"good case": {
			input:     "userID:S",
			wantName:  "userID",
			wantType:  "S",
			wantError: nil,
		},
		"bad case": {
			input:     "userID",
			wantError: fmt.Errorf("parse attribute from key: %s", "userID"),
		},
		"non-ideal input": {
			input:     "userId_cool-table.d:binary",
			wantName:  "userId_cool-table.d",
			wantType:  "B",
			wantError: nil,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, err := DDBAttributeFromKey(tc.input)
			if tc.wantError != nil {
				require.EqualError(t, err, tc.wantError.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.wantName, *got.Name)
				require.Equal(t, tc.wantType, *got.DataType)
			}
		})
	}
}

func TestnewLSI(t *testing.T) {
	testPartitionKey := "Email"
	testSortKey := "Goodness"
	testCases := map[string]struct {
		inPartitionKey string
		inLSIs         []string
		wantedLSI      []DDBLocalSecondaryIndex
		wantError      error
	}{
		"happy case": {
			inPartitionKey: "Email",
			inLSIs:         []string{"Goodness:N"},
			wantedLSI: []DDBLocalSecondaryIndex{
				{
					Name:         &testSortKey,
					PartitionKey: &testPartitionKey,
					SortKey:      &testSortKey,
				},
			},
		},
		"no error getting attribute": {
			inPartitionKey: "Email",
			inLSIs:         []string{"goodness"},
			wantError:      fmt.Errorf("parse attribute from key: goodness"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, err := newLSI(tc.inPartitionKey, tc.inLSIs)
			if tc.wantError != nil {
				require.EqualError(t, err, tc.wantError.Error())
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.wantedLSI, got)
			}
		})
	}
}
