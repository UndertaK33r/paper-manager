package pdfmeta

import "testing"

func lines(ps [][2]any) []Line {
	out := []Line{}
	page := 1
	for _, p := range ps {
		size, _ := p[1].(float64)
		text, _ := p[0].(string)
		out = append(out, Line{Page: page, Size: size, Text: text})
	}
	return out
}

func TestSniffTitleEnglishMultiline(t *testing.T) {
	ls := lines([][2]any{
		{"BERT: Pre-training of Deep Bidirectional Transformers for", 14.3},
		{"Language Understanding", 14.3},
		{"Jacob Devlin Ming-Wei Chang Kenton Lee Kristina Toutanova", 11.5},
		{"Google AI Language", 10},
		{"Abstract", 11},
		{"We introduce a new language representation model called BERT,", 10},
	})
	got := SniffTitleFromLines(ls)
	want := "BERT: Pre-training of Deep Bidirectional Transformers for Language Understanding"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSniffTitleSkipsAuthorsSameSize(t *testing.T) {
	ls := lines([][2]any{
		{"Repurposing Diffusion-Based Image Generators for Monocular Depth Estimation", 17.4},
		{"Bingxin Ke Anton Obukhov Shengyu Huang", 15.2},
		{"Photogrammetry and Remote Sensing, ETH Zurich", 11},
	})
	got := SniffTitleFromLines(ls)
	want := "Repurposing Diffusion-Based Image Generators for Monocular Depth Estimation"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSniffTitleVerticalCJK(t *testing.T) {
	chars := []rune("红外与可见光图像融合：从数据兼容性到任务适配")
	ps := [][2]any{}
	for _, r := range chars {
		ps = append(ps, [2]any{string(r), 24.0})
	}
	ps = append(ps,
		[2]any{"Jinyuan Liu, Member, IEEE, Guanyao Wu", 11},
		[2]any{"摘摘摘", 12},
		[2]any{"要要要", 12},
		[2]any{"红外-可见光图像融合（IVIF）是计算机视觉领域的一个基础性关键的任务", 10.5},
	)
	got := SniffTitleFromLines(lines(ps))
	want := "红外与可见光图像融合：从数据兼容性到任务适配"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSniffTitleRejectsGarbage(t *testing.T) {
	ls := lines([][2]any{
		{"!\"#$%&!\"#", 30},
		{"Some Real Paper Title About Vision Transformers", 24},
	})
	got := SniffTitleFromLines(ls)
	want := "Some Real Paper Title About Vision Transformers"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSniffTitleEmptyWhenNothingPlausible(t *testing.T) {
	ls := lines([][2]any{
		{"!\"#$%&!\"#", 30},
		{"1", 12},
		{"2", 12},
	})
	if got := SniffTitleFromLines(ls); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

func TestPlausibleInfoTitle(t *testing.T) {
	cases := map[string]bool{
		"Deep Residual Learning for Image Recognition": true,
		"Appendix":                                     false,
		"Microsoft Word - final_v3.doc":                false,
		"Untitled":                                     false,
		"":                                             false,
		"!!!!!!!!":                                     false,
	}
	for in, want := range cases {
		got := plausibleInfoTitle(in) != ""
		if got != want {
			t.Errorf("plausibleInfoTitle(%q)=%v want %v", in, got, want)
		}
	}
}

func TestParseToUnicode(t *testing.T) {
	cmap := []byte(`%!PS-Adobe-3.0 Resource-CMap
/CIDInit /ProcSet findresource begin
12 dict begin
begincmap
/CMapName /Adobe-Identity-UCS def
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
2 beginbfchar
<0003> <7EA2>
<0004> <5916>
endbfchar
1 beginbfrange
<0005> <0007> <56FE>
endbfrange
endcmap
CMapName currentdict /CMap defineresource pop
end
end`)
	m := parseToUnicode(cmap)
	if m == nil {
		t.Fatal("parseToUnicode returned nil")
	}
	if m.width != 2 {
		t.Fatalf("width=%d want 2", m.width)
	}
	if s := m.m[0x0003]; s != "红" {
		t.Fatalf("code 3 = %q want 红", s)
	}
	if s := m.m[0x0004]; s != "外" {
		t.Fatalf("code 4 = %q want 外", s)
	}
	// bfrange 连续区间：码点递增（0x56FE=图，0x56FF=囿）
	if s := m.m[0x0005]; s != "图" {
		t.Fatalf("code 5 = %q want 图", s)
	}
	if s := m.m[0x0006]; s != "囿" {
		t.Fatalf("code 6 = %q want 囿", s)
	}
	if s := m.m[0x0007]; s != string(rune(0x5700)) {
		t.Fatalf("code 7 = %q want U+5700", s)
	}
}

func TestFontDecoderTwoByteWithMap(t *testing.T) {
	d := &fontDecoder{
		width: 2,
		tu: &toUnicodeMap{
			width: 2,
			m:     map[uint64]string{0x0003: "红", 0x0004: "外"},
		},
	}
	if got := d.decode("\x00\x03\x00\x04"); got != "红外" {
		t.Fatalf("got %q want 红外", got)
	}
	// 缺失码丢弃，不解码成 UTF-16 乱码
	if got := d.decode("\x00\x03\xFF\xFE"); got != "红" {
		t.Fatalf("got %q want 红", got)
	}
}

func TestSniffTitleFromTextSkipsGarbage(t *testing.T) {
	text := "!\"#$%&!\"#\n쯎찙飮겭쫞쯬\n红外与可见光图像融合研究综述\nAbstract We study fusion\n"
	want := "红外与可见光图像融合研究综述"
	if got := SniffTitle(text); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
