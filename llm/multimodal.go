package llm

// MultimodalClient defines optional helpers for image+text prompts.
// Providers that support vision can implement some or all of these.
// These helpers are additive and do not affect the core Client interface.
type MultimodalClient interface {
	// ChatWithImages sends a single turn that includes images.
	// Implementations should treat imagePaths as local file paths and handle
	// any encoding required by the provider API.
	ChatWithImages(prompt string, imagePaths []string, opts map[string]interface{}) (string, error)

	// StreamChatWithImages streams the response chunks for image+text prompts.
	StreamChatWithImages(prompt string, imagePaths []string, opts map[string]interface{}) (<-chan string, error)
}
