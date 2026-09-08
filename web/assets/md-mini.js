/* ============================================================================
   md-mini.js — 零依赖迷你 Markdown 渲染器
   安全设计：先转义全部 HTML，再从 Markdown 语法生成受控标签，
   因此 AI 输出中的 <script> 等内容只会被显示为文本，不会被执行。
   暴露 window.mdRender(text) -> HTML 字符串。
   ========================================================================== */
(function () {
  "use strict";

  function esc(s) {
    return String(s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function safeUrl(u) {
    u = String(u || "").trim();
    if (/^(https?:\/\/|mailto:|\/|#)/i.test(u)) return u;
    return "";
  }

  // 行内语法：code 保护 → 转义 → 图片/链接/加粗/斜体/删除线 → 还原 code
  function inline(s) {
    var codes = [];
    s = String(s).replace(/`([^`\n]+)`/g, function (_, c) {
      codes.push(c);
      return "\x00" + (codes.length - 1) + "\x00";
    });
    s = esc(s);
    s = s.replace(/!\[([^\]]*)\]\(([^)\s]+)[^)]*\)/g, function (_, alt, u) {
      var su = safeUrl(u);
      return su ? '<img src="' + esc(su) + '" alt="' + alt + '">' : esc(alt);
    });
    s = s.replace(/\[([^\]]+)\]\(([^)\s]+)[^)]*\)/g, function (_, t, u) {
      var su = safeUrl(u);
      return su ? '<a href="' + esc(su) + '" target="_blank" rel="noopener">' + t + "</a>" : t;
    });
    s = s.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>")
         .replace(/__([^_]+)__/g, "<strong>$1</strong>");
    s = s.replace(/(^|[^*\w])\*([^*\n]+)\*/g, "$1<em>$2</em>")
         .replace(/(^|[^\w])_([^_\n]+)_/g, "$1<em>$2</em>");
    s = s.replace(/~~([^~]+)~~/g, "<del>$1</del>");
    s = s.replace(/\x00(\d+)\x00/g, function (_, i) {
      return "<code>" + esc(codes[+i]) + "</code>";
    });
    return s;
  }

  function isHr(line) {
    return /^\s*(?:-{3,}|\*{3,}|_{3,})\s*$/.test(line);
  }
  function isTableSep(line) {
    return /^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)+\|?\s*$/.test(line);
  }
  function splitRow(line) {
    return line.replace(/^\s*\|/, "").replace(/\|\s*$/, "").split("|").map(function (c) { return c.trim(); });
  }

  function render(src) {
    if (!src) return "";
    var lines = String(src).replace(/\r\n?/g, "\n").split("\n");
    var out = [];
    var i, m;

    for (i = 0; i < lines.length; i++) {
      var line = lines[i];

      // 围栏代码块
      if (/^\s*```/.test(line)) {
        var lang = line.replace(/^\s*```/, "").trim();
        var buf = [];
        i++;
        while (i < lines.length && !/^\s*```/.test(lines[i])) { buf.push(lines[i]); i++; }
        out.push('<pre><code' + (lang ? ' class="lang-' + esc(lang) + '"' : "") + ">" + esc(buf.join("\n")) + "</code></pre>");
        continue;
      }
      if (isHr(line)) { out.push("<hr>"); continue; }

      // 标题
      if ((m = line.match(/^\s{0,3}(#{1,4})\s+(.*?)\s*#*\s*$/))) {
        out.push("<h" + m[1].length + ">" + inline(m[2]) + "</h" + m[1].length + ">");
        continue;
      }

      // 引用块（连续 > 行）
      if (/^\s*>/.test(line)) {
        var q = [];
        while (i < lines.length && /^\s*>/.test(lines[i])) {
          q.push(lines[i].replace(/^\s*>\s?/, ""));
          i++;
        }
        i--;
        out.push("<blockquote>" + inline(q.join(" ")) + "</blockquote>");
        continue;
      }

      // 表格（当前行含 |，下一行是分隔行）
      if (line.indexOf("|") >= 0 && i + 1 < lines.length && isTableSep(lines[i + 1])) {
        var head = splitRow(line);
        i += 2;
        var rows = [];
        while (i < lines.length && lines[i].indexOf("|") >= 0 && lines[i].trim() !== "") {
          rows.push(splitRow(lines[i]));
          i++;
        }
        i--;
        var t = '<table><thead><tr>';
        head.forEach(function (c) { t += "<th>" + inline(c) + "</th>"; });
        t += "</tr></thead><tbody>";
        rows.forEach(function (r) {
          t += "<tr>";
          head.forEach(function (_, ci) { t += "<td>" + inline(r[ci] || "") + "</td>"; });
          t += "</tr>";
        });
        t += "</tbody></table>";
        out.push(t);
        continue;
      }

      // 列表（无序列表 -/*/+，有序列表 1.；缩进两层视为同级，连续行聚合）
      if ((m = line.match(/^\s*[-*+]\s+(.*)$/)) || (m = line.match(/^\s*\d+[.)]\s+(.*)$/))) {
        var ordered = /^\s*\d+[.)]/.test(line);
        var items = [];
        while (i < lines.length) {
          var mm = lines[i].match(/^\s*[-*+]\s+(.*)$/) || lines[i].match(/^\s*\d+[.)]\s+(.*)$/);
          if (!mm) break;
          items.push(mm[1]);
          i++;
        }
        i--;
        var li = items.map(function (it) { return "<li>" + inline(it) + "</li>"; }).join("");
        out.push(ordered ? "<ol>" + li + "</ol>" : "<ul>" + li + "</ul>");
        continue;
      }

      // 空行
      if (line.trim() === "") { continue; }

      // 普通段落（连续非空行，行间 <br>）
      var para = [line];
      i++;
      while (i < lines.length && lines[i].trim() !== "" &&
             !/^\s*```/.test(lines[i]) && !/^\s{0,3}#{1,4}\s/.test(lines[i]) &&
             !/^\s*>/.test(lines[i]) && !/^\s*[-*+]\s+/.test(lines[i]) &&
             !/^\s*\d+[.)]\s+/.test(lines[i]) && !isHr(lines[i])) {
        para.push(lines[i]);
        i++;
      }
      i--;
      out.push("<p>" + para.map(inline).join("<br>") + "</p>");
    }
    return out.join("\n");
  }

  window.mdRender = render;
})();
