# macOS Contacts

Settings → Contacts contains a separate macOS Contacts source. Choose **Allow Contacts access** to request macOS permission. Merely opening Settings or typing recipients does not request permission. Once granted, native addresses participate in recipient autocomplete in both the main window and detached composer.

The browser reads names and email addresses (organization is used when a person has no name). Search by name or email, then choose **Browse / refresh**. **Open in Contacts** shows the selected person in Apple's Contacts app; make edits there and refresh the browser afterwards. Native data is read directly rather than bulk-imported into Postilaatikko's local contacts database. Normal sent-recipient history continues to work.

Postilaatikko contains no native contact create, edit, save or delete operation. macOS grants general Contacts access; the implementation uses only read APIs. Revoke access in System Settings → Privacy & Security → Contacts. The next lookup rechecks permission, and other contact sources keep working. Denied or restricted access is explained in Settings, with a check-again action. Platforms without macOS/cgo hide this source.

Reads fetch only the identifiers, names, organization and email addresses needed here. Each native query scans at most 20,000 contacts and returns at most 100 matching people; the browser reports truncation. Autocomplete requests its normal suggestion limit, then merges and deduplicates emails using existing ranking. There is no native background sync, image fetch, contact cache, or native writeback. Changes in Contacts.app appear on the next lookup or browser refresh. Very large address books and names whose first matches have no email may require a more specific search.

Implementation references: [Apple's CNContactStore documentation](https://developer.apple.com/documentation/contacts/cncontactstore), [permission requirements](https://developer.apple.com/documentation/contacts/accessing-the-contact-store), and [opening a record in Contacts.app](https://developer.apple.com/library/archive/documentation/UserExperience/Conceptual/AddressBook/Tasks/URLScheme.html). The record handoff uses Apple's addressbook URL scheme after resolving the identifier against the authorized CNContactStore; it never opens a supplied URL.

## Verification and remaining native acceptance

Automated verification on macOS: permission boundary and store-merge tests (written red first), complete `go test -tags webkit2_41 ./...`, Svelte check, frontend production build and changed-file lint. The non-cgo stub is covered by compiling/running the source tests with `CGO_ENABLED=0`. Both Darwin Info.plist templates provide NSContactsUsageDescription. Tests use a fake source and do not request access to the developer's contacts.

Before release, use a packaged/signed installed app to verify:

1. A fresh launch and recipient typing show no permission dialog. Settings → Contacts → Allow Contacts access presents the system dialog once.
2. Grant access; browse and search contacts (including multiple emails, organization-only names and no-email records). Suggestions work in main-window and detached composers without duplicates.
3. Open a selected person in Contacts.app and confirm the correct record appears; edit there and refresh to see the new value. The URL scheme's behavior with modern CNContact identifiers remains a live-device gate.
4. Deny/revoke/restrict access, refresh the browser and type recipients. No automatic permission prompt occurs; native results disappear and other contact sources keep working. Re-enable access in macOS settings and use Check access.
5. Check keyboard navigation and English/Finnish strings. Other locale files carry English defaults pending translation. Check a large address book for responsiveness and the truncation message.

These native interactions have not been exercised by the automated tests; compilation does not establish TCC permission or Contacts.app handoff behavior.
