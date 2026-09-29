package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLogQuotaDataMergesCacheColumns verifies that per-request quota-data
// writes sharing the same hourly aggregation key accumulate the token and
// cache columns in the in-memory cache that SaveQuotaDataCache later flushes.
func TestLogQuotaDataMergesCacheColumns(t *testing.T) {
	CacheQuotaDataLock.Lock()
	CacheQuotaData = make(map[string]*QuotaData)
	CacheQuotaDataLock.Unlock()

	createdAt := int64(1767225600) // full hour
	base := QuotaDataLogParams{
		UserID:              7,
		Username:            "alice",
		ModelName:           "gpt-test",
		Quota:               10,
		CreatedAt:           createdAt,
		TokenUsed:           100,
		UseGroup:            "default",
		TokenID:             1,
		ChannelID:           2,
		NodeName:            "node-1",
		PromptTokens:        80,
		CacheTokens:         60,
		CacheCreationTokens: 5,
	}
	LogQuotaData(base)

	second := base
	second.Quota = 20
	second.TokenUsed = 50
	second.PromptTokens = 40
	second.CacheTokens = 30
	second.CacheCreationTokens = 0
	// 3599 seconds later still lands in the same hourly bucket.
	second.CreatedAt = createdAt + 3599
	LogQuotaData(second)

	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	require.Len(t, CacheQuotaData, 1)

	var merged *QuotaData
	for _, entry := range CacheQuotaData {
		merged = entry
	}
	require.NotNil(t, merged)
	assert.Equal(t, 2, merged.Count)
	assert.Equal(t, 30, merged.Quota)
	assert.Equal(t, 150, merged.TokenUsed)
	assert.Equal(t, 120, merged.PromptTokens)
	assert.Equal(t, 90, merged.CacheTokens)
	assert.Equal(t, 5, merged.CacheCreationTokens)
}

// TestLogQuotaDataSeparatesDistinctHours verifies cache columns do not bleed
// across different hourly buckets.
func TestLogQuotaDataSeparatesDistinctHours(t *testing.T) {
	CacheQuotaDataLock.Lock()
	CacheQuotaData = make(map[string]*QuotaData)
	CacheQuotaDataLock.Unlock()

	LogQuotaData(QuotaDataLogParams{
		UserID:              7,
		Username:            "alice",
		ModelName:           "gpt-test",
		Quota:               1,
		CreatedAt:           1767225600,
		TokenUsed:           10,
		PromptTokens:        10,
		CacheTokens:         4,
		CacheCreationTokens: 2,
	})
	LogQuotaData(QuotaDataLogParams{
		UserID:              7,
		Username:            "alice",
		ModelName:           "gpt-test",
		Quota:               2,
		CreatedAt:           1767225600 + 3600,
		TokenUsed:           20,
		PromptTokens:        20,
		CacheTokens:         8,
		CacheCreationTokens: 6,
	})

	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	require.Len(t, CacheQuotaData, 2)

	totalCache, totalCreation := 0, 0
	for _, entry := range CacheQuotaData {
		totalCache += entry.CacheTokens
		totalCreation += entry.CacheCreationTokens
	}
	assert.Equal(t, 12, totalCache)
	assert.Equal(t, 8, totalCreation)
}
