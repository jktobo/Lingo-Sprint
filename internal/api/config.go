package api

// JwtKey is the secret key used for signing and verifying JWT tokens.
// IMPORTANT: In a real application, load this from environment variables/config file!
var JwtKey = []byte("my_very_secret_and_long_key_32_bytes")