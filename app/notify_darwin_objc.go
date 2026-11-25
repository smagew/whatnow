//go:build darwin && cgo

package main

// The Objective-C side of notify_darwin.go. It lives in a file of its own: a cgo file that exports a Go function
// (notifyClickedC, in notify_darwin.go) may only declare C functions in its preamble, not define them.

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework AppKit -framework UserNotifications
#import <Foundation/Foundation.h>
#import <AppKit/AppKit.h>
#import <UserNotifications/UserNotifications.h>

extern void notifyClickedC(char *slug);

@interface WNNotifyDelegate : NSObject <UNUserNotificationCenterDelegate>
@end
@implementation WNNotifyDelegate
// shown even while the app is in front (the dashboard decides itself when a notification is wanted)
- (void)userNotificationCenter:(UNUserNotificationCenter *)c willPresentNotification:(UNNotification *)n
         withCompletionHandler:(void (^)(UNNotificationPresentationOptions))done {
	done(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionList | UNNotificationPresentationOptionSound);
}
- (void)userNotificationCenter:(UNUserNotificationCenter *)c didReceiveNotificationResponse:(UNNotificationResponse *)r
         withCompletionHandler:(void (^)(void))done {
	NSString *slug = r.notification.request.content.userInfo[@"slug"];
	notifyClickedC((char *)[(slug ?: @"") UTF8String]);
	done();
}
@end

static WNNotifyDelegate *wnDelegate;
static volatile int wnState = 0; // 0 not known yet, 1 allowed, -1 refused or not possible

int wnNotifyInit(void) {
	if ([[NSBundle mainBundle] bundleIdentifier] == nil) { wnState = -1; return 0; } // not inside whatnow.app
	UNUserNotificationCenter *c = [UNUserNotificationCenter currentNotificationCenter];
	wnDelegate = [WNNotifyDelegate new];
	c.delegate = wnDelegate;
	[c requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
	                 completionHandler:^(BOOL granted, NSError *error) { wnState = granted ? 1 : -1; }];
	return 1;
}

int wnNotifyState(void) { return wnState; }

int wnNotifyPost(const char *title, const char *sub, const char *body, const char *slug, int sound) {
	if (wnState != 1) return 0;
	UNMutableNotificationContent *n = [UNMutableNotificationContent new];
	n.title = [NSString stringWithUTF8String:title];
	n.subtitle = [NSString stringWithUTF8String:sub];
	n.body = [NSString stringWithUTF8String:body];
	n.userInfo = @{ @"slug": [NSString stringWithUTF8String:slug] };
	if (sound) n.sound = [UNNotificationSound defaultSound];
	UNNotificationRequest *r = [UNNotificationRequest requestWithIdentifier:[[NSUUID UUID] UUIDString] content:n trigger:nil];
	[[UNUserNotificationCenter currentNotificationCenter] addNotificationRequest:r withCompletionHandler:nil];
	return 1;
}
*/
import "C"
