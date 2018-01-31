package common_test

import (
	"context"
	"testing"

	"gopkg.in/jarcoal/httpmock.v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code.uber.internal/engsec/wonka-go.git/wonkamaster/common"
)

func TestGetPoolsForHost(t *testing.T) {
	mockRes := `{"Data": [{"Name": "somePool"}]}`
	expectedRes := map[string]struct{}{"somepool": {}}
	url := "localhost:17949/dapi/trusteng/GetParentsForServer/dummyURL"
	httpmock.Activate()
	httpmock.RegisterResponder("GET", url, httpmock.NewStringResponder(200, mockRes))
	resp, err := common.GetPoolsForHost(context.Background(), "dummyURL")
	assert.Equal(t, expectedRes, resp)
	require.Nil(t, err)
	httpmock.DeactivateAndReset()
}

func TestGetPoolsForHostHandlesErrors(t *testing.T) {
	httpmock.Activate()
	resp, err := common.GetPoolsForHost(context.Background(), "dummyURL")
	assert.Nil(t, resp)
	require.NotNil(t, err)
	httpmock.DeactivateAndReset()
}
