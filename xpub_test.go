package loop_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightninglabs/loop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const testXpub = "xpub661MyMwAqRbcFtXgS5sYJABqqG9YLmC4Q1Rdap9gSE8NqtwybGhePY2gZ29ESFjqJoCu1Rupje8YtGqsefD265TMg7usUDFdp6W1EGMcet8"

func TestGetNextAddressForXpub(t *testing.T) {

	mockXpubStore := new(MockXpubStore)

	service := loop.NewXpubService(mockXpubStore, &chaincfg.MainNetParams)

	ctx := context.Background()

	mockXpubStore.On("GetNextExternalIndex", ctx, testXpub).Return(uint32(1), nil)

	address, err := service.GetNextAddressForXpub(ctx, testXpub)

	assert.NoError(t, err)
	assert.NotNil(t, address)

	fmt.Println(address.String())
}

type MockXpubStore struct {
	mock.Mock
}

func (m *MockXpubStore) GetNextExternalIndex(ctx context.Context, xpub string) (uint32, error) {
	args := m.Called(ctx, xpub)
	return args.Get(0).(uint32), args.Error(1)
}
