const std = @import("std");
const fs = std.fs;
const log = std.log;
const rivit = @import("rivit");

pub const std_options = struct {
    pub const log_level = .info;
    pub const logFn = loggerFn;
};

const site_name = "beton brutalism";
const in_path = "riv";
const out_path = "docs";
const res_path = "res";
const base_url = "https://github.com/judah-caruso/judah-caruso.github.io";
const template = @embedFile("template.htm");

// 100mb max memory (shouldn't have to worry about this ever)
const max_memory: usize = (1024 * 1024) * 100;

var arena: std.mem.Allocator = undefined;

pub fn main() !void {
    const sys = std.heap.page_allocator;
    var mem = sys.alloc(u8, max_memory) catch {
        log.err("unable to allocate program memory!", .{});
        std.os.exit(1);
    };

    defer sys.free(mem);

    var a = std.heap.FixedBufferAllocator.init(mem);
    arena = a.allocator();

    std.debug.print("\n", .{}); // build output doesn't have \n

    const cwd = fs.cwd();
    var wiki_dir = cwd.openIterableDir(in_path, .{}) catch {
        log.err("required directory '{s}' doesn't exist!", .{in_path});
        std.os.exit(2);
    };
    defer wiki_dir.close();

    var res_dir = cwd.openIterableDir(res_path, .{}) catch {
        log.err("required directory '{s}' doesn't exist!", .{res_path});
        std.os.exit(3);
    };
    defer res_dir.close();

    const styles = res_dir.dir.readFileAlloc(arena, "style.css", max_memory) catch {
        log.err("required stylesheet 'style.css' doesn't exist!", .{});
        std.os.exit(3);
    };

    const stylesheet = std.mem.replaceOwned(u8, arena, styles, "\n", "") catch @panic("out of memory!");
    arena.free(styles);

    var index = Index.init();

    // index all riv files (needed so we can verify links)
    var dir_iter = wiki_dir.iterate();
    while (try dir_iter.next()) |f| {
        const ext = ".riv";
        if (f.kind != .file or !std.mem.endsWith(u8, f.name, ext)) {
            continue;
        }

        var ctime: i128 = 0;
        if (wiki_dir.dir.statFile(f.name)) |info| {
            ctime = info.ctime;
        } else |_| {
            ctime = std.time.nanoTimestamp();
            log.warn("unable to get creation date for '{s}'! using current time instead.", .{f.name});
        }

        index.addToIndex(f.name, ctime);
        log.debug("indexed '{s}'", .{f.name});
    }

    // setup output path
    var generated: usize = 0;
    var out_dir = cwd.makeOpenPath(out_path, .{}) catch {
        log.err("unable to create/open output directory '{s}'!", .{out_path});
        std.os.exit(4);
    };

    defer out_dir.close();

    // parse and output each indexed file
    var page_iter = index.pages.iterator();
    while (page_iter.next()) |kv| {
        var page = kv.value_ptr;
        const sys_path = page.local_path;

        var src = wiki_dir.dir.readFileAlloc(arena, sys_path, std.math.maxInt(usize)) catch {
            log.err("unable to open page '{s}'! skipping...", .{sys_path});
            continue;
        };

        page.body = rivit.parse(arena, src) catch {
            log.err("unable to parse '{s}'! skipping...", .{sys_path});
            continue;
        };

        var out_file = out_dir.createFile(page.out_path, .{}) catch {
            log.err("unable to create output page '{s}'! skipping...", .{page.out_path});
            continue;
        };

        defer out_file.close();

        // unix -> yymmdd conversion
        const created = c: {
            // jesus christ
            const s = @divFloor(page.create_time, std.time.ns_per_s);
            const z = @divFloor(s, 86400) + 719468;
            const era = @divFloor(if (z >= 0) z else z - 146096, 146097);
            const doe: u128 = @intCast(z - era * 146097);
            const yoe: u128 = @intCast(@divFloor(doe - @divFloor(doe, 1460) + @divFloor(doe, 36524) - @divFloor(doe, 146096), 365));
            var year: i128 = @as(i128, @intCast(yoe)) + era * 400;
            const doy = doe - (365 * yoe + @divFloor(yoe, 4) - @divFloor(yoe, 100));
            const mp = @divFloor(5 * doy + 2, 153);
            const day = doy - @divFloor(153 * mp + 2, 5) + 1;
            var month = mp;
            if (mp < 10) month += 3 else month -= 9;
            if (month <= 2) year += 2;

            break :c std.fmt.allocPrint(arena, "{d}{d:0>2}{d:0>2}", .{ year - 2000, month, day }) catch @panic("out of memory!");
        };

        // setup nav links
        for (page.body.lines.items) |item| {
            if (item != .nav_link) continue;

            const name = item.nav_link;
            if (index.getPage(name)) |p| {
                page.addNavLink(p);
            } else {
                log.warn("'{s}' has a broken nav link '{s}'! skipping...", .{ page.local_path, name });
            }
        }

        std.sort.insertion(*Page, page.nav_links.items, {}, Page.sortByName);

        // generate nav list html
        var nav = std.ArrayList(u8).init(arena);
        defer nav.deinit();

        {
            var writer = nav.writer();
            try writer.writeAll("<ul class='list'>");

            for (page.nav_links.items) |p| {
                try writer.writeAll("<li class='list-item'>");
                try writer.print("<a class='internal link' href='{s}'>{s}</a>", .{
                    p.out_path,
                    p.display_name,
                });
                try writer.writeAll("</li>");
            }

            try writer.writeAll("</ul>");
        }

        // output rivit as html
        var body = std.ArrayList(u8).init(arena);
        defer body.deinit();

        {
            var writer = body.writer();
            for (page.body.lines.items, 0..) |item, idx| {
                if (item == .nav_link) continue;

                switch (item) {
                    .paragraph => |p| {
                        try writer.writeAll("<p class='paragraph'>");
                        try styledTextToHtml(&index, page, &writer, p);
                        try writer.writeAll("</p>");
                    },

                    .header => |h| {
                        const level: usize = if (idx == 0) 1 else 2;
                        try writer.print("<h{[0]} class='header'>{[1]s}</h{[0]}>", .{ level, h });
                    },

                    .block => |b| {
                        try writer.writeAll("<pre class='code'>");

                        var lines = std.mem.split(u8, b.body, "\n");
                        while (lines.next()) |l| {
                            var line = l;
                            if (b.indent < line.len) {
                                line = l[b.indent..];
                            }

                            for (0..line.len) |i| {
                                const chr = line[i];
                                try switch (chr) {
                                    '<' => writer.writeAll("&lt;"),
                                    '>' => writer.writeAll("&gt;"),
                                    else => writer.writeByte(chr),
                                };
                            }

                            if (lines.index != null) {
                                try writer.writeAll("\n");
                            }
                        }

                        try writer.writeAll("</pre>");
                    },

                    .list => |l| {
                        try listToHtml(&index, page, &writer, l);
                    },

                    .embed => |e| {
                        const ext = std.fs.path.extension(e.path);

                        var media_type = MediaType.unknown;
                        if (std.mem.eql(u8, ext, ".png")) {
                            media_type = .png;
                        } else if (std.mem.eql(u8, ext, ".ogg")) {
                            media_type = .ogg;
                        } else if (std.mem.eql(u8, ext, ".svg")) {
                            media_type = .svg;
                        } else {
                            log.warn("'{s}' references an unsupported media type '{s}'! skipping...", .{ page.local_path, ext });
                            continue;
                        }

                        const file = res_dir.dir.readFileAlloc(arena, e.path, max_memory) catch |err| {
                            if (err == error.FileNotFound) {
                                log.warn("'{s}' references media '{s}' that doesn't exist! skipping...", .{ page.local_path, e.path });
                                continue;
                            }

                            log.err("'{s}' references media '{s}' that is too large! skipping...", .{ page.local_path, e.path });
                            continue;
                        };

                        defer arena.free(file);

                        const encoder = std.base64.standard.Encoder;
                        const size = encoder.calcSize(file.len);

                        const buf = arena.alloc(u8, size) catch @panic("out of memory!");
                        const b64 = encoder.encode(buf, file);

                        try writer.writeAll("<figure>");

                        // image files
                        switch (media_type) {
                            .png => {
                                const prefix = "data:image/png;base64";
                                try writer.print("<img class='image' src='{s},{s}'/>", .{ prefix, b64 });
                            },
                            .svg => {
                                const prefix = "data:image/svg+xml;base64";
                                try writer.print("<img class='vector' src='{s},{s}'/>", .{ prefix, b64 });
                            },
                            .ogg => {
                                const prefix = "data:audio/ogg;base64";
                                try writer.print("<audio class='sound' loop controls autobuffer src='{s}, {s}'></audio>", .{ prefix, b64 });
                            },
                            else => unreachable,
                        }

                        if (e.alt_text) |alt| {
                            try writer.writeAll("<figcaption>");
                            try styledTextToHtml(&index, page, &writer, alt);
                            try writer.writeAll("</figcaption>");
                        }

                        try writer.writeAll("</figure>");
                    },

                    else => unreachable,
                }
            }
        }

        // write final file
        var writer = out_file.writer();
        try writer.print(template, .{
            .title = site_name,
            .name = page.display_name,
            .nav = nav.items,
            .body = body.items,
            .style = stylesheet,
            .created = created,
            .edit_url = std.fmt.allocPrint(arena, "{s}/edit/main/{s}/{s}", .{ base_url, in_path, page.local_path }) catch @panic("out of memory!"),
        });

        generated += 1;
    }

    // log orphaned pages
    page_iter.index = 0;
    while (page_iter.next()) |kv| {
        var page = kv.value_ptr.*;
        if (page.refs == 0 and !std.mem.eql(u8, page.local_path, "index.riv")) {
            log.warn("orphaned page '{s}'", .{page.local_path});
        }
    }

    std.debug.print("\n", .{});
    log.info("generated {} {s}", .{ generated, if (generated == 1) "page" else "pages" });
}

fn listToHtml(index: *Index, page: *Page, writer: anytype, list: std.ArrayList(rivit.Line.ListItem)) !void {
    try writer.writeAll("<ul class='list'>");

    for (list.items) |li| {
        try writer.writeAll("<li class='list-item'>");
        try styledTextToHtml(index, page, writer, li.value);

        if (li.sublist) |sublist| {
            try listToHtml(index, page, writer, sublist);
        }

        try writer.writeAll("</li>");
    }

    try writer.writeAll("</ul>");
}

fn styledTextToHtml(index: *Index, page: *Page, writer: anytype, text: std.ArrayList(rivit.StyledText)) !void {
    for (text.items) |t| {
        switch (t) {
            .unstyled => |u| try writer.writeAll(u),
            .bold => |b| try writer.print("<strong class='bold'>{s}</strong>", .{b}),
            .italic => |i| try writer.print("<em class='italic'>{s}</em>", .{i}),
            .escaped => |e| switch (e) {
                '*' => try writer.writeAll("&times;"),
                '{' => try writer.writeAll("&#123;"),
                '[' => try writer.writeAll("&#91;"),
                else => try writer.print("{c}", .{e}),
            },
            .internal_link => |i| {
                try writer.writeAll("<a ");

                var fallback_name = i.name;
                if (index.getPage(i.name)) |p| {
                    try writer.writeAll("class='internal link' ");
                    try writer.print("href='{s}'>", .{p.out_path});

                    fallback_name = p.display_name;
                } else {
                    log.warn("'{s}' has a broken internal link '{s}'", .{ page.local_path, i.name });

                    try writer.writeAll("class='broken internal link' target='_blank' ");
                    try writer.print("href='{s}/new/main/{s}?filename={s}.riv'>", .{
                        base_url,
                        in_path,
                        i.name,
                    });
                }

                if (i.value) |v| {
                    try writer.writeAll(v);
                } else {
                    try writer.writeAll(fallback_name);
                }

                try writer.writeAll("</a>");
            },
            .external_link => |e| {
                const v = e.value orelse e.url;
                try writer.print("<a class='external link' target='_blank' href='{s}'>{s}</a>", .{ e.url, v });
            },
        }
    }
}

const Index = struct {
    pages: PageMap,

    const Self = @This();
    const PageMap = std.StringHashMap(Page);

    pub fn init() Self {
        return .{
            .pages = PageMap.init(arena),
        };
    }

    pub fn addToIndex(self: *Self, file_path: []const u8, create_time: i128) void {
        const page = Page.create(file_path, create_time);
        self.pages.put(page.unique_id, page) catch @panic("unable to index page");
    }

    pub fn tryRef(self: *Self, name: []const u8) ?*Page {
        if (self.pages.getPtr(name)) |page| {
            page.refs += 1;
            return page;
        }

        return null;
    }

    pub fn getPage(self: *Self, name: []const u8) ?*Page {
        if (self.pages.getPtr(name)) |p| return p;
        return null;
    }
};

const Page = struct {
    refs: usize, // how many pages reference this page
    body: rivit.Rivit,
    nav_links: std.ArrayList(*Page),
    create_time: i128,

    unique_id: []const u8, // path without extension
    display_name: []const u8, // user-facing name
    local_path: []const u8, // path to .riv file
    out_path: []const u8, // path to .htm file

    const Self = @This();

    pub fn create(local_path: []const u8, create_time: i128) Self {
        const ext = fs.path.extension(local_path);
        const id = local_path[0 .. local_path.len - ext.len];
        const display = std.mem.replaceOwned(u8, arena, id, "-", " ") catch @panic("out of memory!");
        const out = std.fmt.allocPrint(arena, "{s}.htm", .{id}) catch @panic("out of memory!");

        return .{
            .refs = 0,
            .unique_id = id,
            .create_time = create_time,
            .display_name = display,
            .local_path = local_path,
            .out_path = out,
            .body = undefined,
            .nav_links = std.ArrayList(*Self).init(arena),
        };
    }

    pub fn addNavLink(self: *Self, page: *Page) void {
        var exists = false;
        for (self.nav_links.items) |ex| if (std.mem.eql(u8, page.unique_id, ex.unique_id)) {
            exists = true;
            break;
        };

        if (!exists) {
            self.nav_links.append(page) catch @panic("unable to push nav link");
        }
    }

    pub fn sortByName(ctx: void, a: *const Self, b: *const Self) bool {
        _ = ctx;
        return (a.unique_id.len > 0 and b.unique_id.len > 0) and (a.unique_id[0] < b.unique_id[0]);
    }
};

const MediaType = enum {
    unknown,
    png,
    ogg,
    svg,
};

fn loggerFn(
    comptime level: std.log.Level,
    comptime scope: @TypeOf(.EnumLiteral),
    comptime format: []const u8,
    args: anytype,
) void {
    _ = scope;

    const color = switch (level) {
        .debug => "34",
        .info => "37",
        .warn => "33",
        else => "31",
    };

    const prefix = ".. \x1b[1:" ++ color ++ "m" ++ comptime level.asText() ++ "\x1b[0m ";

    std.debug.getStderrMutex().lock();
    defer std.debug.getStderrMutex().unlock();

    const stderr = std.io.getStdErr().writer();
    nosuspend stderr.print(prefix ++ format ++ "\n", args) catch return;
}
