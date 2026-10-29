//go:build darwin && cgo

#import <Contacts/Contacts.h>
#import <AppKit/AppKit.h>
#include <stdlib.h>
#include <string.h>

int plContactsStatus(void) {
    return (int)[CNContactStore authorizationStatusForEntityType:CNEntityTypeContacts];
}

char *plContactsRequest(void) {
    @autoreleasepool {
        // Missing usage descriptions crash the process rather than return an NSError.
        NSString *usage = [[NSBundle mainBundle] objectForInfoDictionaryKey:@"NSContactsUsageDescription"];
        if (usage.length == 0) return strdup("Use the packaged app to grant Contacts access.");
        dispatch_semaphore_t finished = dispatch_semaphore_create(0);
        __block NSString *failure = nil;
        dispatch_async(dispatch_get_main_queue(), ^{
            CNContactStore *store = [[CNContactStore alloc] init];
            [store requestAccessForEntityType:CNEntityTypeContacts completionHandler:^(BOOL granted, NSError *error) {
                if (error) failure = error.localizedDescription;
                dispatch_semaphore_signal(finished);
            }];
        });
        // Called from a Wails Go worker, never from the AppKit main thread.
        dispatch_semaphore_wait(finished, DISPATCH_TIME_FOREVER);
        return failure ? strdup(failure.UTF8String) : NULL;
    }
}

char *plContactsSearch(const char *query, int limit) {
    @autoreleasepool {
        // Recheck at the last boundary: enumeration can itself trigger a prompt.
        int status = plContactsStatus();
        if (status != 3 && status != 4) return strdup("{\"error\":\"Contacts access is not authorized\"}");
        CNContactStore *store = [[CNContactStore alloc] init];
        NSArray *keys = @[CNContactIdentifierKey, CNContactGivenNameKey, CNContactFamilyNameKey,
                          CNContactOrganizationNameKey, CNContactEmailAddressesKey];
        CNContactFetchRequest *request = [[CNContactFetchRequest alloc] initWithKeysToFetch:keys];
        request.sortOrder = CNContactSortOrderUserDefault;
        NSString *needle = [NSString stringWithUTF8String:query];
        NSMutableArray *people = [NSMutableArray array];
        __block NSUInteger scanned = 0;
        __block BOOL truncated = NO;
        NSError *error = nil;
        BOOL ok = [store enumerateContactsWithFetchRequest:request error:&error usingBlock:^(CNContact *contact, BOOL *stop) {
            if (++scanned > 20000) { truncated = YES; *stop = YES; return; }
            NSString *name = [[@[contact.givenName, contact.familyName] componentsJoinedByString:@" "]
                              stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet];
            if (!name.length) name = contact.organizationName;
            NSMutableArray *emails = [NSMutableArray array];
            BOOL matches = !needle.length || [name localizedCaseInsensitiveContainsString:needle];
            for (CNLabeledValue<NSString *> *email in contact.emailAddresses) {
                [emails addObject:email.value];
                if ([email.value localizedCaseInsensitiveContainsString:needle]) matches = YES;
            }
            if (!matches) return;
            if (people.count >= limit) {truncated = YES; *stop = YES; return;}
            [people addObject:@{@"id":contact.identifier, @"name":name, @"emails":emails}];
        }];
        NSDictionary *response = ok ? @{@"people":people, @"truncated":@(truncated)}
                                    : @{@"error":error.localizedDescription ?: @"Unable to read Contacts"};
        NSData *json = [NSJSONSerialization dataWithJSONObject:response options:0 error:&error];
        if (!json) return strdup("{\"error\":\"Unable to encode Contacts\"}");
        return strdup([[NSString alloc] initWithData:json encoding:NSUTF8StringEncoding].UTF8String);
    }
}

char *plContactsOpen(const char *identifier) {
    @autoreleasepool {
        int status = plContactsStatus();
        if (status != 3 && status != 4) return strdup("Contacts access is not authorized");
        NSString *contactID = [NSString stringWithUTF8String:identifier];
        NSError *error = nil;
        CNContactStore *store = [[CNContactStore alloc] init];
        // Resolve against the authorized store, never open an arbitrary supplied URL.
        CNContact *person = [store unifiedContactWithIdentifier:contactID keysToFetch:@[CNContactIdentifierKey] error:&error];
        if (!person) return strdup((error.localizedDescription ?: @"Contact no longer exists").UTF8String);
        NSCharacterSet *allowed = [NSCharacterSet characterSetWithCharactersInString:@"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"];
        NSString *escaped = [person.identifier stringByAddingPercentEncodingWithAllowedCharacters:allowed];
        NSURL *url = [NSURL URLWithString:[@"addressbook://" stringByAppendingString:escaped]];
        __block BOOL opened = NO;
        dispatch_sync(dispatch_get_main_queue(), ^{opened = [[NSWorkspace sharedWorkspace] openURL:url];});
        return opened ? NULL : strdup("Unable to open Contacts.app");
    }
}
