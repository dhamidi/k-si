// Package mime is käsi's object model (docs/02): everything that crosses a
// boundary or gets archived — inbound mail, outbound replies, agent inputs and
// outputs — is a MIME message or MIME part. It leans on the standard library
// (net/mail, mime, mime/multipart, mime/quotedprintable) and stays pure: no
// clock, no randomness, so Build is byte-for-byte reproducible and replay is
// stable (docs/13).
package mime
