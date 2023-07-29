const std = @import("std");
const fs = std.fs;
const log = std.log;

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

// 10mb max memory
const max_memory: usize = (1024 * 1024) * 10;

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

    // index all pages
    var dir_iter = wiki_dir.iterate();
    while (try dir_iter.next()) |f| {
        const ext = ".riv";
        if (f.kind != .file or !std.mem.endsWith(u8, f.name, ext)) {
            continue;
        }

        index.addToIndex(f.name);

        log.debug("indexed '{s}'", .{f.name});
    }

    var generated: usize = 0;
    var out_dir = cwd.makeOpenPath(out_path, .{}) catch {
        log.err("unable to create/open output directory '{s}'!", .{out_path});
        std.os.exit(4);
    };

    defer out_dir.close();

    var page_iter = index.pages.iterator();
    while (page_iter.next()) |kv| {
        var page = kv.value_ptr;
        const sys_path = page.local_path;

        var src = wiki_dir.dir.readFileAlloc(arena, sys_path, std.math.maxInt(usize)) catch {
            log.err("unable to open page '{s}'! skipping...", .{sys_path});
            continue;
        };

        parseRivitSource(&index, page, src, res_dir.dir);

        var out_file = out_dir.createFile(page.out_path, .{}) catch {
            log.err("unable to create output page '{s}'! skipping...", .{page.out_path});
            continue;
        };

        defer out_file.close();

        // unix -> yymmdd conversion
        const created = c: {
            const info = try out_file.stat();

            // jesus christ
            const s = @divFloor(info.ctime, std.time.ns_per_s);
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

        var nav = std.ArrayList(u8).init(arena);
        // std.sort.insertion(Text, page.nav_links.items, {}, Text.sort);

        {
            var writer = nav.writer();
            try writer.writeAll("<ul class='list'>");

            for (page.nav_links.items) |link| {
                try writer.print("<li class='list-item'>{}</li>\n", .{link});
            }

            try writer.writeAll("</ul>");
        }

        var writer = out_file.writer();
        try writer.print(template, .{
            .title = site_name,
            .name = page.display_name,
            .nav = nav.items,
            .body = std.fmt.allocPrint(arena, "{}", .{page}) catch @panic("out of memory!"),
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

const Index = struct {
    pages: PageMap,

    const Self = @This();
    const PageMap = std.StringHashMap(Page);

    pub fn init() Self {
        return .{
            .pages = PageMap.init(arena),
        };
    }

    pub fn addToIndex(self: *Self, file_path: []const u8) void {
        const page = Page.create(file_path);
        self.pages.put(page.unique_id, page) catch @panic("unable to index page");
    }

    pub fn tryRef(self: *Self, name: []const u8) ?*Page {
        if (self.pages.getPtr(name)) |page| {
            page.refs += 1;
            return page;
        }

        return null;
    }
};

const Page = struct {
    refs: usize, // how many pages reference this page
    body: std.ArrayList(Rivit),
    nav_links: std.ArrayList(Text), // always internal_link

    unique_id: []const u8, // path without extension
    display_name: []const u8, // user-facing name
    local_path: []const u8, // path to .riv file
    out_path: []const u8, // path to .htm file

    const Self = @This();

    pub fn create(local_path: []const u8) Self {
        const ext = fs.path.extension(local_path);
        const id = local_path[0 .. local_path.len - ext.len];
        const display = std.mem.replaceOwned(u8, arena, id, "-", " ") catch @panic("out of memory!");
        const out = std.fmt.allocPrint(arena, "{s}.htm", .{id}) catch @panic("out of memory!");

        return .{
            .refs = 0,
            .unique_id = id,
            .display_name = display,
            .local_path = local_path,
            .out_path = out,
            .body = std.ArrayList(Rivit).init(arena),
            .nav_links = std.ArrayList(Text).init(arena),
        };
    }

    pub fn append(self: *Self, r: Rivit) void {
        self.body.append(r) catch @panic("unable to push node");
    }

    pub fn addNavLink(self: *Self, l: Text) void {
        std.debug.assert(l.style == .internal_link);

        var exists = false;
        for (self.nav_links.items) |existing| if (std.mem.eql(u8, l.extra, existing.extra)) {
            exists = true;
            break;
        };

        if (!exists) self.nav_links.append(l) catch @panic("unable to push node");
    }

    pub fn format(
        self: *const Self,
        comptime fmt: []const u8,
        options: std.fmt.FormatOptions,
        writer: anytype,
    ) !void {
        _ = fmt;
        _ = options;

        for (self.body.items) |r| {
            try writer.print("{}\n", .{r});
        }
    }
};

const MediaKind = enum {
    unsupported,
    image,
    audio,
};

const RivitKind = enum {
    // Styled Rivits
    paragraph,
    list,

    // Structured Rivits
    header,
    media,
    code,
};
const Rivit = union(RivitKind) {
    paragraph: struct {
        body: std.ArrayList(Text),
    },
    list: struct {
        body: std.ArrayList(std.ArrayList(Text)),
    },

    header: struct {
        is_title: bool,
        body: []const u8,
    },
    media: struct {
        kind: MediaKind,
        base64: []const u8,
        caption: []const u8,
    },
    code: struct {
        body: []const u8,
        indent: usize,
    },

    const Self = @This();

    pub fn format(
        self: *const Self,
        comptime fmt: []const u8,
        options: std.fmt.FormatOptions,
        writer: anytype,
    ) !void {
        _ = fmt;
        _ = options;

        switch (self.*) {
            .paragraph => |p| {
                try writer.writeAll("<p class='paragraph'>");

                for (p.body.items) |text| {
                    try writer.print("{}", .{text});
                }

                try writer.writeAll("</p>");
            },
            .list => |l| {
                try writer.writeAll("<ul class='list'>");

                for (l.body.items) |list| {
                    try writer.writeAll("<li class='list-item'>");

                    for (list.items) |item| {
                        try writer.print("{}", .{item});
                    }

                    try writer.writeAll("</li>");
                }

                try writer.writeAll("</ul>");
            },
            .header => |h| {
                const level: usize = if (h.is_title) 1 else 2;
                try writer.print("<h{[0]} class='header'>{[1]s}</h{[0]}>", .{ level, h.body });
            },
            .media => |m| {
                try writer.writeAll("<figure>");

                switch (m.kind) {
                    .image => {
                        const prefix = "data:image/png;base64";
                        if (m.caption.len > 0) {
                            try writer.print("<img alt='{s}' src='{s}, {s}'/>", .{ m.caption, prefix, m.base64 });
                        } else {
                            try writer.print("<img src='{s}, {s}'/>", .{ prefix, m.base64 });
                        }
                    },
                    .audio => {
                        const prefix = "data:audio/wav;base64";
                        try writer.print("<audio controls autobuffer src='{s}, {s}'></audio>", .{ prefix, m.base64 });
                    },
                    else => {},
                }

                if (m.caption.len > 0) {
                    try writer.print("<figcaption>{s}</figcaption>", .{m.caption});
                }

                try writer.writeAll("</figure>");
            },
            .code => |c| {
                try writer.writeAll("<pre class='code'>");

                var lines = std.mem.split(u8, c.body, "\n");
                while (lines.next()) |l| {
                    var line = l;
                    if (c.indent < line.len) {
                        line = l[c.indent..];
                    }

                    line = std.mem.replaceOwned(u8, arena, line, "<", "&lt;") catch @panic("out of memory!");
                    line = std.mem.replaceOwned(u8, arena, line, ">", "&gt;") catch @panic("out of memory!");
                    try writer.writeAll(line);

                    if (lines.index != null) {
                        try writer.writeAll("\n");
                    }
                }

                try writer.writeAll("</pre>");
            },
        }
    }
};

const Text = struct {
    style: Style,
    value: []const u8,
    extra: []const u8, // url if style == .internal/external_link

    // used for internal_links
    broken: bool = false,

    const Style = enum {
        none,
        bold,
        italic,
        internal_link,
        external_link,
    };

    const Self = @This();

    pub fn sort(ctx: void, a: Self, b: Self) bool {
        _ = ctx;
        return (a.value.len > 0 and b.value.len > 0) and (a.value[0] < b.value[0]);
    }

    pub fn format(
        self: *const Self,
        comptime fmt: []const u8,
        options: std.fmt.FormatOptions,
        writer: anytype,
    ) !void {
        _ = fmt;
        _ = options;

        switch (self.style) {
            .bold => {
                try writer.print("<strong class='bold'>{s}</strong>", .{self.value});
            },
            .italic => {
                try writer.print("<em class='italic'>{s}</em>", .{self.value});
            },
            .internal_link => {
                const class = if (self.broken) "broken " else "";
                const target = if (self.broken) " target='_blank' " else "";
                try writer.print("<a class='{s}internal link'{s}href='{s}'>{s}</a>", .{ class, target, self.extra, self.value });
            },
            .external_link => {
                const v = if (self.value.len > 0) self.value else self.extra;
                try writer.print("<a class='external link' target='_blank' href='{s}'>{s}</a>", .{ self.extra, v });
            },
            else => {
                try writer.writeAll(self.value);
            },
        }
    }
};

fn parseRivitSource(index: *Index, page: *Page, src: []const u8, res_dir: std.fs.Dir) void {
    var lines = std.mem.splitAny(u8, src, "\n");
    while (lines.next()) |l| {
        var line = std.mem.trimRight(u8, l, " \t\r");
        if (line.len == 0) continue;

        const delim = line[0];
        switch (delim) {
            // comment
            '\\' => continue,

            // code block
            '\t', ' ' => {
                const block_start = lines.index.? - (l.len + 1);

                const block_indent = i: {
                    var indent: usize = 0;

                    var tmp = line;
                    while (tmp.len > 0) {
                        if (tmp[0] != delim) break;
                        tmp = tmp[1..];
                        indent += 1;
                    }

                    break :i indent;
                };

                while (lines.next()) |next_line| {
                    var indent: usize = 0;

                    var tmp = next_line;
                    while (tmp.len > 0) {
                        if (tmp[0] != delim) break;
                        tmp = tmp[1..];
                        indent += 1;
                    }

                    if (indent < block_indent) {
                        if (lines.index) |*i| i.* -= 1;
                        break;
                    }
                }

                const block_end = lines.index orelse lines.buffer.len;
                const code_block = lines.buffer[block_start..block_end];

                page.append(.{
                    .code = .{
                        .body = code_block,
                        .indent = block_indent,
                    },
                });
            },

            // list item
            // @todo: this definitely doesn't handle nested lists due to
            // the structure of Rivit/Text. could be done easily but I'm lazy.
            '.' => {
                var level = l: {
                    var level: usize = 0;

                    var tmp = line;
                    while (tmp.len > 0) {
                        if (tmp[0] != '.') break;
                        tmp = tmp[1..];
                        level += 1;
                    }

                    break :l level;
                };

                var list = std.ArrayList(std.ArrayList(Text)).init(arena);
                list.append(std.ArrayList(Text).init(arena)) catch @panic("unable to push node");

                parseText(index, page, &list.items[0], std.mem.trimLeft(u8, line[level..], " \t"));

                while (lines.next()) |nl| {
                    if (nl.len == 0 or nl[0] != '.') {
                        if (lines.index) |*i| i.* -= 1;
                        break;
                    }

                    var sub_level: usize = 0;

                    var tmp = nl;
                    while (tmp.len > 0) {
                        if (tmp[0] != '.') break;
                        tmp = tmp[1..];
                        sub_level += 1;
                    }

                    if (sub_level >= level) {
                        list.append(std.ArrayList(Text).init(arena)) catch @panic("unable to push node");

                        const text = std.mem.trim(u8, nl[sub_level..], " \t");
                        parseText(index, page, &list.items[list.items.len - 1], text);
                    }
                }

                page.append(.{
                    .list = .{
                        .body = list,
                    },
                });
            },

            // media
            '@' => {
                line = std.mem.trimLeft(u8, line[1..], " \t");

                const path = p: {
                    if (std.mem.indexOf(u8, line, " ")) |idx| {
                        const path = line[0..idx];
                        line = std.mem.trimLeft(u8, line[idx..], " \t");
                        break :p path;
                    } else {
                        const path = line;
                        line = "";
                        break :p path;
                    }
                };

                if (path.len <= 0) {
                    continue;
                }

                const ext = std.fs.path.extension(path);
                const kind: MediaKind = t: {
                    if (std.mem.eql(u8, ext, ".png")) {
                        break :t .image;
                    }

                    if (std.mem.eql(u8, ext, ".wav")) {
                        break :t .audio;
                    }

                    break :t .unsupported;
                };

                if (kind == .unsupported) {
                    log.warn("'{s}' has unsupported media '{s}'", .{ page.local_path, path });
                    continue;
                }

                const file = res_dir.readFileAlloc(arena, path, max_memory) catch |err| {
                    if (err == error.FileNotFound) {
                        log.warn("'{s}' references media '{s}' that doesn't exist!", .{ page.local_path, path });
                        continue;
                    }

                    log.err("'{s}' references media '{s}' that is too large! ignoring...", .{ page.local_path, path });
                    continue;
                };

                defer arena.free(file);

                const encoder = std.base64.standard.Encoder;
                const size = encoder.calcSize(file.len);

                const buf = arena.alloc(u8, size) catch @panic("out of memory!");
                const b64 = encoder.encode(buf, file);

                page.append(.{
                    .media = .{
                        .kind = kind,
                        .caption = line,
                        .base64 = b64,
                    },
                });
            },

            else => {
                // header
                if (std.ascii.isAlphabetic(line[0])) {
                    var tmp = line;
                    while (tmp.len > 0) {
                        if (std.ascii.isLower(tmp[0])) break;
                        tmp = tmp[1..];
                    }

                    if (tmp.len == 0) {
                        page.append(.{ .header = .{ .body = line, .is_title = page.body.items.len == 0 } });
                        continue;
                    }
                }

                // regular text
                if (line.len > 0) {
                    var p = Rivit{
                        .paragraph = .{ .body = std.ArrayList(Text).init(arena) },
                    };

                    parseText(index, page, &p.paragraph.body, line);

                    if (p.paragraph.body.items.len > 0) {
                        page.append(p);
                    }
                }
            },
        }
    }
}

fn parseText(index: *Index, page: *Page, body: *std.ArrayList(Text), line: []const u8) void {
    var i: usize = 0;
    while (i < line.len) {
        switch (line[i]) {
            // italics, bold
            '*' => {
                var chunk_start = i;
                var chunk_end = i;

                var style = Text.Style.italic;

                // bold
                if (chunk_end + 1 < line.len and line[chunk_end + 1] == '*') {
                    style = .bold;

                    chunk_end += 1;
                    while (chunk_end < line.len) : (chunk_end += 1) {
                        if (line[chunk_end] == '*') {
                            if (chunk_end + 1 < line.len and line[chunk_end + 1] == '*') {
                                chunk_end += 2;
                                break;
                            }
                        }
                    }
                }
                // italic
                else {
                    chunk_end += 1;
                    while (chunk_end < line.len) : (chunk_end += 1) {
                        if (line[chunk_end] == '*') {
                            chunk_end += 1;
                            break;
                        }
                    }
                }

                const text = std.mem.trim(
                    u8,
                    if (style == .italic)
                        line[chunk_start + 1 .. chunk_end - 1]
                    else
                        line[chunk_start + 2 .. chunk_end - 2],
                    " \t",
                );

                if (text.len > 0) {
                    body.append(.{
                        .style = style,
                        .value = text,
                        .extra = "",
                    }) catch @panic("out of memory!");
                }

                i = chunk_end;
            },

            // nav links
            '>' => {
                const link = std.mem.trimLeft(u8, line[1..], " \t");
                if (index.tryRef(link)) |ref| {
                    page.addNavLink(.{
                        .style = .internal_link,
                        .value = ref.display_name,
                        .extra = ref.out_path,
                    });
                } else {
                    log.warn("'{s}' has a broken nav link '{s}'", .{ page.local_path, link });
                }

                i += line.len;
            },

            // internal links
            '{' => {
                var chunk_start = i;
                var chunk_end = i;
                while (chunk_end < line.len) : (chunk_end += 1) {
                    if (line[chunk_end] == '}') {
                        chunk_end += 1;
                        break;
                    }
                }

                const link = std.mem.trim(u8, line[chunk_start + 1 .. chunk_end - 1], " \t");
                if (link.len > 0) {
                    if (index.tryRef(link)) |ref| {
                        body.append(.{
                            .style = .internal_link,
                            .value = ref.display_name,
                            .extra = ref.out_path,
                        }) catch @panic("out of memory");

                        page.addNavLink(body.items[body.items.len - 1]);
                    } else {
                        log.warn("'{s}' has a broken internal link '{s}'", .{ page.local_path, link });

                        const create_link = std.fmt.allocPrint(
                            arena,
                            "{s}/new/main/{s}?filename={s}.riv",
                            .{
                                base_url,
                                in_path,
                                link,
                            },
                        ) catch @panic("out of memory!");

                        body.append(.{
                            .style = .internal_link,
                            .value = link,
                            .extra = create_link,
                            .broken = true,
                        }) catch @panic("out of memory!");
                    }
                }

                i = chunk_end;
            },

            // external links
            '[' => {
                var chunk_start = i;
                var chunk_end = i;
                while (chunk_end < line.len) : (chunk_end += 1) {
                    if (line[chunk_end] == ']') {
                        chunk_end += 1;
                        break;
                    }
                }

                var link = std.mem.trim(u8, line[chunk_start + 1 .. chunk_end - 1], " \t");
                var text = if (std.mem.indexOf(u8, link, " ")) |idx| v: {
                    const val = std.mem.trimLeft(u8, link[idx..], " \t");
                    link = std.mem.trimRight(u8, link[0..idx], " \t");
                    break :v val;
                } else "";

                if (link.len > 0) {
                    body.append(.{
                        .style = .external_link,
                        .value = text,
                        .extra = link,
                    }) catch @panic("out of memory!");
                }

                i = chunk_end;
            },

            else => {
                var chunk_start = i;
                var chunk_end = i;
                while (chunk_end < line.len) {
                    switch (line[chunk_end]) {
                        '*', '{', '[' => break,
                        else => chunk_end += 1,
                    }
                }

                const text = line[chunk_start..chunk_end];
                if (text.len > 0) {
                    body.append(.{
                        .style = .none,
                        .value = text,
                        .extra = "",
                    }) catch @panic("out of memory!");
                }

                i = chunk_end;
            },
        }
    }
}

pub fn loggerFn(
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
