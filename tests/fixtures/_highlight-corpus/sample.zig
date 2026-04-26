// Zig sample
const std = @import("std");

const Counter = struct {
    value: usize = 0,

    pub fn increment(self: *Counter, n: usize) usize {
        self.value += n;
        return self.value;
    }
};

pub fn main() !void {
    var gpa = std.heap.GeneralPurposeAllocator(.{}){};
    defer _ = gpa.deinit();
    const allocator = gpa.allocator();

    const path = "input.txt";
    const file = try std.fs.cwd().openFile(path, .{});
    defer file.close();

    var buf_reader = std.io.bufferedReader(file.reader());
    var reader = buf_reader.reader();

    var counter = Counter{};
    var line_buf = std.ArrayList(u8).init(allocator);
    defer line_buf.deinit();

    while (true) {
        line_buf.clearRetainingCapacity();
        reader.streamUntilDelimiter(line_buf.writer(), '\n', null) catch |err| switch (err) {
            error.EndOfStream => break,
            else => return err,
        };
        if (line_buf.items.len > 0) {
            _ = counter.increment(line_buf.items.len);
        }
    }

    const stdout = std.io.getStdOut().writer();
    try stdout.print("total bytes: {d}\n", .{counter.value});
}
