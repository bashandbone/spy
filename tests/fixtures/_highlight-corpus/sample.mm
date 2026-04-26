// Objective-C++ sample
#import <Foundation/Foundation.h>

#include <string>
#include <vector>

@interface Counter : NSObject
@property (nonatomic, readonly) NSInteger value;
- (NSInteger)increment:(NSInteger)n;
@end

@implementation Counter {
    NSInteger _value;
}
- (instancetype)init {
    if ((self = [super init])) {
        _value = 0;
    }
    return self;
}
- (NSInteger)value { return _value; }
- (NSInteger)increment:(NSInteger)n {
    _value += n;
    return _value;
}
@end

int main(int argc, const char *argv[]) {
    @autoreleasepool {
        std::vector<std::string> lines;
        NSString *path = (argc > 1) ? @(argv[1]) : @"input.txt";
        NSString *text = [NSString stringWithContentsOfFile:path
                                                   encoding:NSUTF8StringEncoding
                                                      error:nil];
        Counter *counter = [[Counter alloc] init];
        for (NSString *line in [text componentsSeparatedByString:@"\n"]) {
            std::string s = std::string([line UTF8String]);
            if (!s.empty()) {
                [counter increment:(NSInteger)s.size()];
            }
        }
        printf("total bytes: %ld\n", (long)counter.value);
    }
    return 0;
}
