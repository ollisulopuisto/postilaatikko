# Host-independent extension access

This package extracts the desktop's extension Auth and secret adapters for reuse by an owner-trusted headless helper. It does not add API routes, grant device access, provision a remote host, copy credentials, or enable remote Contacts/Calendar by itself.

Construct Auth with the existing auth.Broker and the compiled extension's trusted manifest/identity. Never accept the identity, manifest or namespace from an HTTP caller. The adapter snapshots routing slices; tokens remain inside the broker. Interactive consent is unavailable unless the host supplies an explicit consent callback. A headless host must not pretend consent succeeded or launch an unattended browser flow.

Construct Secrets with the existing credentials.Store and a fixed extension namespace. HostSecrets permits the existing read-only CardDAV password class only. These are in-process secret handles, never JSON response objects or an arbitrary secret-export API. The existing store's keyring/encrypted fallback behavior is unchanged.

Contacts and Calendar services still need separate API capabilities, authenticated routing, provider/source authorization, bounded requests, mutation receipts and lifecycle shutdown. Wiring these adapters is not runtime parity or a security audit. Native keychain and real two-machine/provider validation remain required.
