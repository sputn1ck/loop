package swapdk

// NOTE: gomobile automatically exposes exported functions/types.  These thin
// wrappers make the API a bit prettier from Java/Kotlin/Swift but are optional.

// GenerateNewContextMobile wraps GenerateNewContext for gomobile so callers get
// a pointer they can hold onto.
func GenerateNewContextMobile() (*Context, error) { return GenerateNewContext() }

// NewContextFromMnemonicMobile wraps NewContextFromMnemonic.
func NewContextFromMnemonicMobile(m string) (*Context, error) { return NewContextFromMnemonic(m) }

// NewClientMobile wraps NewClient.
func NewClientMobile(serverAddr string) *Client {
	return NewClient(serverAddr)
}
