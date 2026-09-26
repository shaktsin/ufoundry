#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#import <CoreGraphics/CoreGraphics.h>
#import <ImageIO/ImageIO.h>

static NSDictionary *readInput(void) {
    NSData *data = [[NSFileHandle fileHandleWithStandardInput] readDataToEndOfFile];
    if (data.length == 0) return @{};
    id value = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
    return [value isKindOfClass:[NSDictionary class]] ? value : @{};
}

static void writeJSON(id value) {
    NSData *data = [NSJSONSerialization dataWithJSONObject:value options:0 error:nil];
    [[NSFileHandle fileHandleWithStandardOutput] writeData:data];
}

static void fail(NSString *message) {
    fprintf(stderr, "%s\n", message.UTF8String);
    exit(1);
}

static NSDictionary *appJSON(NSRunningApplication *app) {
    return @{ @"name": app.localizedName ?: @"",
              @"bundle_id": app.bundleIdentifier ?: @"",
              @"pid": @(app.processIdentifier),
              @"path": app.bundleURL.path ?: @"" };
}

static NSRunningApplication *findApp(NSDictionary *target) {
    pid_t pid = [target[@"pid"] intValue];
    NSString *bundle = target[@"bundle_id"] ?: @"";
    NSString *name = target[@"name"] ?: @"";
    NSString *path = target[@"path"] ?: @"";
    for (NSRunningApplication *app in NSWorkspace.sharedWorkspace.runningApplications) {
        if ((pid > 0 && app.processIdentifier == pid) ||
            (bundle.length && [app.bundleIdentifier isEqualToString:bundle]) ||
            (name.length && [app.localizedName caseInsensitiveCompare:name] == NSOrderedSame) ||
            (path.length && [app.bundleURL.path isEqualToString:path])) return app;
    }
    return nil;
}

static NSDictionary *windowForPID(pid_t pid) {
    CFArrayRef raw = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
    NSArray *windows = CFBridgingRelease(raw);
    for (NSDictionary *window in windows) {
        if ([window[(id)kCGWindowOwnerPID] intValue] != pid || [window[(id)kCGWindowLayer] intValue] != 0) continue;
        CGRect bounds = CGRectZero;
        CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)window[(id)kCGWindowBounds], &bounds);
        if (bounds.size.width < 2 || bounds.size.height < 2) continue;
        return @{ @"id": window[(id)kCGWindowNumber] ?: @0,
                  @"title": window[(id)kCGWindowName] ?: @"",
                  @"x": @(bounds.origin.x), @"y": @(bounds.origin.y),
                  @"width": @(bounds.size.width), @"height": @(bounds.size.height) };
    }
    return nil;
}

static NSDictionary *captureWindow(NSDictionary *window, NSString *path) {
    if (!CGPreflightScreenCaptureAccess() && !CGRequestScreenCaptureAccess()) {
        fail(@"Screen Recording permission is required. Enable UMCode Computer Use in System Settings → Privacy & Security → Screen Recording.");
    }
    CGWindowID windowID = [window[@"id"] unsignedIntValue];
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
    CGImageRef image = CGWindowListCreateImage(CGRectNull, kCGWindowListOptionIncludingWindow, windowID, kCGWindowImageBoundsIgnoreFraming);
#pragma clang diagnostic pop
    if (!image) fail(@"Could not capture the selected application window.");
    size_t pixelWidth = CGImageGetWidth(image), pixelHeight = CGImageGetHeight(image);
    NSURL *url = [NSURL fileURLWithPath:path];
    CGImageDestinationRef dest = CGImageDestinationCreateWithURL((__bridge CFURLRef)url, CFSTR("public.png"), 1, NULL);
    if (!dest) { CGImageRelease(image); fail(@"Could not create the screenshot artifact."); }
    CGImageDestinationAddImage(dest, image, NULL);
    BOOL ok = CGImageDestinationFinalize(dest);
    CFRelease(dest);
    CGImageRelease(image);
    if (!ok) fail(@"Could not write the screenshot artifact.");
    return @{ @"pixel_width": @(pixelWidth), @"pixel_height": @(pixelHeight) };
}

static NSArray *accessibilityControls(pid_t pid) {
    AXUIElementRef app = AXUIElementCreateApplication(pid);
    CFTypeRef windows = NULL;
    NSMutableArray *out = [NSMutableArray array];
    if (AXUIElementCopyAttributeValue(app, kAXWindowsAttribute, &windows) == kAXErrorSuccess && windows) {
        for (id item in (__bridge NSArray *)windows) {
            AXUIElementRef window = (__bridge AXUIElementRef)item;
            CFTypeRef title = NULL;
            if (AXUIElementCopyAttributeValue(window, kAXTitleAttribute, &title) == kAXErrorSuccess && title) {
                [out addObject:[NSString stringWithFormat:@"window: %@", (__bridge id)title]];
                CFRelease(title);
            }
            CFTypeRef children = NULL;
            if (AXUIElementCopyAttributeValue(window, kAXChildrenAttribute, &children) == kAXErrorSuccess && children) {
                NSUInteger count = 0;
                for (id child in (__bridge NSArray *)children) {
                    if (count++ >= 80) break;
                    AXUIElementRef element = (__bridge AXUIElementRef)child;
                    CFTypeRef role = NULL, label = NULL, value = NULL;
                    AXUIElementCopyAttributeValue(element, kAXRoleAttribute, &role);
                    AXUIElementCopyAttributeValue(element, kAXTitleAttribute, &label);
                    AXUIElementCopyAttributeValue(element, kAXValueAttribute, &value);
                    NSString *line = [NSString stringWithFormat:@"%@: %@%@%@",
                        role ? (__bridge id)role : @"element",
                        label ? (__bridge id)label : @"",
                        (label && value) ? @" · " : @"",
                        value ? [(__bridge id)value description] : @""];
                    if (line.length > 2) [out addObject:line];
                    if (role) CFRelease(role); if (label) CFRelease(label); if (value) CFRelease(value);
                }
                CFRelease(children);
            }
        }
        CFRelease(windows);
    }
    CFRelease(app);
    return out;
}

static void postClick(CGPoint point, BOOL twice) {
    for (int i = 1; i <= (twice ? 2 : 1); i++) {
        CGEventRef down = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseDown, point, kCGMouseButtonLeft);
        CGEventRef up = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseUp, point, kCGMouseButtonLeft);
        CGEventSetIntegerValueField(down, kCGMouseEventClickState, i);
        CGEventSetIntegerValueField(up, kCGMouseEventClickState, i);
        CGEventPost(kCGHIDEventTap, down); CGEventPost(kCGHIDEventTap, up);
        CFRelease(down); CFRelease(up);
    }
}

static void enterText(NSString *text) {
    NSUInteger length = text.length;
    UniChar *chars = calloc(length, sizeof(UniChar));
    [text getCharacters:chars range:NSMakeRange(0, length)];
    CGEventRef down = CGEventCreateKeyboardEvent(NULL, 0, true);
    CGEventRef up = CGEventCreateKeyboardEvent(NULL, 0, false);
    CGEventKeyboardSetUnicodeString(down, length, chars);
    CGEventKeyboardSetUnicodeString(up, length, chars);
    CGEventPost(kCGHIDEventTap, down); CGEventPost(kCGHIDEventTap, up);
    CFRelease(down); CFRelease(up); free(chars);
}

static CGKeyCode keyCode(NSString *key) {
    NSDictionary *codes = @{ @"return": @36, @"enter": @36, @"tab": @48, @"space": @49,
        @"escape": @53, @"left": @123, @"right": @124, @"down": @125, @"up": @126,
        @"delete": @51, @"backspace": @51, @"home": @115, @"end": @119 };
    NSNumber *code = codes[key.lowercaseString];
    return code ? code.unsignedShortValue : UINT16_MAX;
}

static void pressKey(NSString *key) {
    NSString *lower = key.lowercaseString;
    if ([lower isEqualToString:@"cmd+a"] || [lower isEqualToString:@"command+a"]) {
        CGEventRef down = CGEventCreateKeyboardEvent(NULL, 0, true);
        CGEventRef up = CGEventCreateKeyboardEvent(NULL, 0, false);
        CGEventSetFlags(down, kCGEventFlagMaskCommand); CGEventSetFlags(up, kCGEventFlagMaskCommand);
        CGEventPost(kCGHIDEventTap, down); CGEventPost(kCGHIDEventTap, up);
        CFRelease(down); CFRelease(up); return;
    }
    CGKeyCode code = keyCode(lower);
    if (code == UINT16_MAX) fail([NSString stringWithFormat:@"Unsupported key: %@", key]);
    CGEventRef down = CGEventCreateKeyboardEvent(NULL, code, true);
    CGEventRef up = CGEventCreateKeyboardEvent(NULL, code, false);
    CGEventPost(kCGHIDEventTap, down); CGEventPost(kCGHIDEventTap, up);
    CFRelease(down); CFRelease(up);
}

int main(int argc, const char *argv[]) {
    @autoreleasepool {
        if (argc < 2) fail(@"Expected a Computer Use helper command.");
        NSString *command = [NSString stringWithUTF8String:argv[1]];
        NSDictionary *input = readInput();
        if ([command isEqualToString:@"list"]) {
            NSMutableArray *apps = [NSMutableArray array];
            for (NSRunningApplication *app in NSWorkspace.sharedWorkspace.runningApplications) {
                if (app.activationPolicy == NSApplicationActivationPolicyRegular && app.localizedName.length) [apps addObject:appJSON(app)];
            }
            writeJSON(@{ @"apps": apps }); return 0;
        }
        NSDictionary *target = input[@"target"] ?: @{};
        NSRunningApplication *app = findApp(target);
        if (!app) fail(@"The selected application is not running.");
        [app activateWithOptions:0];
        NSDictionary *window = windowForPID(app.processIdentifier);
        if (!window) fail(@"The selected application has no visible window.");
        if ([command isEqualToString:@"inspect"]) {
            NSString *path = input[@"screenshot"];
            if (!path.length) fail(@"A screenshot path is required.");
            NSDictionary *pixels = captureWindow(window, path);
            NSMutableDictionary *windowState = [window mutableCopy];
            [windowState addEntriesFromDictionary:pixels];
            BOOL accessibility = AXIsProcessTrusted();
            writeJSON(@{ @"app": appJSON(app), @"window": windowState,
                         @"permission": accessibility ? @"ready" : @"screen-only; Accessibility permission is required for actions",
                         @"controls": accessibility ? accessibilityControls(app.processIdentifier) : @[] });
            return 0;
        }
        if (![command isEqualToString:@"act"]) fail(@"Unknown Computer Use helper command.");
        NSDictionary *options = @{(__bridge id)kAXTrustedCheckOptionPrompt: @YES};
        if (!AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef)options)) {
            fail(@"Accessibility permission is required. Enable UMCode Computer Use in System Settings → Privacy & Security → Accessibility.");
        }
        NSDictionary *action = input[@"action"] ?: @{};
        NSString *type = action[@"type"] ?: @"";
        CGPoint point = CGPointMake([window[@"x"] doubleValue] + [action[@"x"] doubleValue],
                                    [window[@"y"] doubleValue] + [action[@"y"] doubleValue]);
        if ([type isEqualToString:@"click"] || [type isEqualToString:@"double_click"] || [type isEqualToString:@"fill"]) {
            postClick(point, [type isEqualToString:@"double_click"]);
        }
        if ([type isEqualToString:@"fill"]) { pressKey(@"cmd+a"); enterText(action[@"text"] ?: @""); }
        else if ([type isEqualToString:@"type"]) enterText(action[@"text"] ?: @"");
        else if ([type isEqualToString:@"key"]) pressKey(action[@"key"] ?: @"");
        else if ([type isEqualToString:@"scroll"]) {
            CGEventRef event = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitPixel, 1, [action[@"delta"] intValue]);
            CGEventPost(kCGHIDEventTap, event); CFRelease(event);
        } else if (![type isEqualToString:@"click"] && ![type isEqualToString:@"double_click"] && ![type isEqualToString:@"fill"]) {
            fail(@"Unsupported Computer Use action.");
        }
        writeJSON(@{ @"ok": @YES });
    }
    return 0;
}
