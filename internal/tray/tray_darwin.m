#import <Cocoa/Cocoa.h>
#include "tray.h"

@interface ProximityTray : NSObject
@property (strong) NSStatusItem *statusItem;
@end

@implementation ProximityTray
- (void)onShow:(id)sender { tray_on_show(); }
- (void)onQuit:(id)sender { tray_on_quit(); }
@end

static ProximityTray *gTray = nil;

void tray_start(const char *iconBytes, int iconLen) {
	// Capture bytes before the block runs asynchronously — caller's buffer
	// may have been GC'd by the time the main queue picks this up.
	NSData *data = (iconBytes != NULL && iconLen > 0)
		? [NSData dataWithBytes:iconBytes length:iconLen]
		: nil;

	dispatch_async(dispatch_get_main_queue(), ^{
		if (gTray != nil) {
			return;
		}
		gTray = [[ProximityTray alloc] init];
		// NSSquareStatusItemLength gives a stable one-menu-bar-height square.
		// NSVariableStatusItemLength collapses to ~0pt when the image's rep is
		// huge (e.g. a 1024x1024 PNG), making the item invisible/unclickable.
		gTray.statusItem = [[NSStatusBar systemStatusBar]
			statusItemWithLength:NSSquareStatusItemLength];

		NSImage *image = nil;

		// Primary: the bundle's own appicon.png. Matches the Dock icon and
		// `/Applications/Proximity.app` bundle icon so the user gets a single
		// consistent brand mark.
		if (data != nil) {
			image = [[NSImage alloc] initWithData:data];
			if (image != nil) {
				[image setSize:NSMakeSize(18, 18)];
				// Template mode tints by alpha and erases the source colors —
				// bad for a colorful app icon. Keep it off so the icon
				// renders in its actual colors.
				[image setTemplate:NO];
			}
		}

		// Fallback: SF Symbol "sparkles" if the PNG fails to decode (corrupt
		// asset, unsupported format, etc.).
		if (image == nil) {
			if (@available(macOS 11.0, *)) {
				image = [NSImage imageWithSystemSymbolName:@"sparkles"
					accessibilityDescription:@"Proximity"];
				if (image != nil) {
					NSImageSymbolConfiguration *cfg = [NSImageSymbolConfiguration
						configurationWithPointSize:16
						                    weight:NSFontWeightRegular];
					image = [image imageWithSymbolConfiguration:cfg];
				}
			}
		}

		if (image != nil) {
			gTray.statusItem.button.image = image;
			gTray.statusItem.button.imageScaling = NSImageScaleProportionallyDown;
		} else {
			gTray.statusItem.button.title = @"✦";
		}
		gTray.statusItem.button.toolTip = @"Proximity";

		NSMenu *menu = [[NSMenu alloc] init];

		NSMenuItem *showItem = [[NSMenuItem alloc]
			initWithTitle:@"Show Proximity"
			       action:@selector(onShow:)
			keyEquivalent:@""];
		[showItem setTarget:gTray];
		[menu addItem:showItem];

		[menu addItem:[NSMenuItem separatorItem]];

		NSMenuItem *quitItem = [[NSMenuItem alloc]
			initWithTitle:@"Quit Proximity"
			       action:@selector(onQuit:)
			keyEquivalent:@"q"];
		[quitItem setTarget:gTray];
		[menu addItem:quitItem];

		gTray.statusItem.menu = menu;

		// Dock visibility is not set here — main.go drives it via
		// tray_set_dock_visible() to mirror window visibility (standard
		// menu-bar-app behavior). On launch the window is visible, so we
		// leave Wails' default Regular policy alone.
	});
}

void tray_set_dock_visible(int visible) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (visible) {
			[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
			// Without this, the window can come back behind other apps —
			// uncommon, but jarring when it happens.
			[NSApp activateIgnoringOtherApps:YES];
		} else {
			[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
		}
	});
}

void tray_stop(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (gTray != nil && gTray.statusItem != nil) {
			[[NSStatusBar systemStatusBar] removeStatusItem:gTray.statusItem];
			gTray.statusItem = nil;
		}
		gTray = nil;
	});
}
