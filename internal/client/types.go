package client

// RecordType values supported by this webhook.
const RecordTypeTXT = "txt"

// txtValue is the ArvanCloud representation of a TXT record payload.
type txtValue struct {
	Text string `json:"text"`
}

// createRecordRequest is the body sent when creating a DNS record.
type createRecordRequest struct {
	Type  string   `json:"type"`
	Name  string   `json:"name"`
	Value txtValue `json:"value"`
	TTL   int      `json:"ttl"`
}

// Record is a single DNS record as returned by the ArvanCloud API.
type Record struct {
	ID    string      `json:"id"`
	Type  string      `json:"type"`
	Name  string      `json:"name"`
	TTL   int         `json:"ttl"`
	Value RecordValue `json:"value"`
}

// RecordValue tolerates the two shapes ArvanCloud uses for a record value:
// an object ({"text": "..."}) for TXT records and, defensively, a bare string.
type RecordValue struct {
	Text string
}

// listRecordsResponse is the paginated list payload.
type listRecordsResponse struct {
	Data []Record `json:"data"`
	Meta struct {
		CurrentPage int `json:"current_page"`
		LastPage    int `json:"last_page"`
	} `json:"meta"`
}

// apiError is the error envelope returned by the ArvanCloud API.
type apiError struct {
	Message string              `json:"message"`
	Errors  map[string][]string `json:"errors"`
}
