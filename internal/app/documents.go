package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/xuri/excelize/v2"
	"io"
	"math"
	"os"
	"rsc.io/pdf"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

func xmlText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
func writeZip(path string, entries map[string]string) error {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	z := zip.NewWriter(f)
	names := make([]string, 0, len(entries))
	for n := range entries {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		w, e := z.Create(n)
		if e != nil {
			z.Close()
			f.Close()
			return e
		}
		if _, e = io.WriteString(w, entries[n]); e != nil {
			z.Close()
			f.Close()
			return e
		}
	}
	e = z.Close()
	ce := f.Close()
	if e == nil {
		e = ce
	}
	return e
}

const xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`
const relNS = `http://schemas.openxmlformats.org/package/2006/relationships`
const officeNS = `http://schemas.openxmlformats.org/officeDocument/2006/relationships`

func docxFile(path string, d Deliverable) error {
	para := func(text, style string) string {
		return `<w:p><w:pPr><w:pStyle w:val="` + style + `"/></w:pPr><w:r><w:t xml:space="preserve">` + xmlText(text) + `</w:t></w:r></w:p>`
	}
	body := para(d.Title, "Title")
	for _, s := range d.Sections {
		if s.Heading != "" {
			body += para(s.Heading, "Heading1")
		}
		for _, line := range strings.Split(s.Body, "\n") {
			body += para(line, "Normal")
		}
	}
	return writeZip(path, map[string]string{
		"[Content_Types].xml":          xmlHeader + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/></Types>`,
		"_rels/.rels":                  xmlHeader + `<Relationships xmlns="` + relNS + `"><Relationship Id="rId1" Type="` + officeNS + `/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/_rels/document.xml.rels": xmlHeader + `<Relationships xmlns="` + relNS + `"><Relationship Id="rId1" Type="` + officeNS + `/styles" Target="styles.xml"/></Relationships>`,
		"word/styles.xml":              xmlHeader + `<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:eastAsia="Microsoft YaHei"/><w:sz w:val="22"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="160" w:line="300" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style><w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:basedOn w:val="Normal"/><w:rPr><w:b/><w:color w:val="203D70"/><w:sz w:val="40"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="260" w:after="140"/><w:outlineLvl w:val="0"/></w:pPr><w:rPr><w:b/><w:color w:val="203D70"/><w:sz w:val="28"/></w:rPr></w:style></w:styles>`,
		"word/document.xml":            xmlHeader + `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body + `<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1134" w:right="1134" w:bottom="1134" w:left="1134"/></w:sectPr></w:body></w:document>`,
	})
}
func pptxFile(path string, d Deliverable) error {
	sections := []Section{{Heading: d.Title, Body: "OpsBuddy · 工作汇报"}}
	sections = append(sections, d.Sections...)
	// Split long sections into bounded pages; editable text is preserved.
	pages := []Section{}
	for _, s := range sections {
		r := []rune(s.Body)
		if len(r) == 0 {
			pages = append(pages, s)
			continue
		}
		for i := 0; i < len(r); i += 280 {
			end := i + 280
			if end > len(r) {
				end = len(r)
			}
			title := s.Heading
			if i > 0 {
				title += "（续）"
			}
			pages = append(pages, Section{title, string(r[i:end])})
		}
	}
	entries := map[string]string{}
	entries["_rels/.rels"] = xmlHeader + `<Relationships xmlns="` + relNS + `"><Relationship Id="rId1" Type="` + officeNS + `/officeDocument" Target="ppt/presentation.xml"/></Relationships>`
	types := `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/><Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/><Override PartName="/ppt/slideLayouts/slideLayout1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"/><Override PartName="/ppt/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>`
	pns := `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="` + officeNS + `" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"`
	group := `<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>`
	shape := func(num int, text string, y, h, size int, color string) string {
		paras := ""
		for _, line := range strings.Split(text, "\n") {
			paras += fmt.Sprintf(`<a:p><a:pPr marL="0" indent="0"><a:lnSpc><a:spcPct val="125000"/></a:lnSpc></a:pPr><a:r><a:rPr lang="zh-CN" sz="%d"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="Calibri"/><a:ea typeface="Microsoft YaHei"/></a:rPr><a:t>%s</a:t></a:r><a:endParaRPr lang="zh-CN"/></a:p>`, size, color, xmlText(line))
		}
		return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="Text %d"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="850000" y="%d"/><a:ext cx="10400000" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/></p:spPr><p:txBody><a:bodyPr wrap="square"><a:normAutofit/></a:bodyPr><a:lstStyle/>%s</p:txBody></p:sp>`, num, num, y, h, paras)
	}
	ids := ""
	rels := `<Relationships xmlns="` + relNS + `"><Relationship Id="rId1" Type="` + officeNS + `/slideMaster" Target="slideMasters/slideMaster1.xml"/>`
	for i, s := range pages {
		n := i + 1
		types += fmt.Sprintf(`<Override PartName="/ppt/slides/slide%d.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`, n)
		ids += fmt.Sprintf(`<p:sldId id="%d" r:id="rId%d"/>`, 256+i, n+1)
		rels += fmt.Sprintf(`<Relationship Id="rId%d" Type="%s/slide" Target="slides/slide%d.xml"/>`, n+1, officeNS, n)
		entries[fmt.Sprintf("ppt/slides/slide%d.xml", n)] = xmlHeader + `<p:sld ` + pns + `><p:cSld><p:bg><p:bgPr><a:solidFill><a:srgbClr val="F5F7FB"/></a:solidFill><a:effectLst/></p:bgPr></p:bg><p:spTree>` + group + shape(2, s.Heading, 620000, 1000000, 3000, "203D70") + shape(3, s.Body, 1850000, 3900000, 2000, "334155") + shape(4, fmt.Sprintf("OPSBUDDY                                            %02d / %02d", n, len(pages)), 6170000, 300000, 1000, "6B7D99") + `</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`
		entries[fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", n)] = xmlHeader + `<Relationships xmlns="` + relNS + `"><Relationship Id="rId1" Type="` + officeNS + `/slideLayout" Target="../slideLayouts/slideLayout1.xml"/></Relationships>`
	}
	entries["[Content_Types].xml"] = xmlHeader + types + `</Types>`
	entries["ppt/presentation.xml"] = xmlHeader + `<p:presentation ` + pns + `><p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst><p:sldIdLst>` + ids + `</p:sldIdLst><p:sldSz cx="12192000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/></p:presentation>`
	entries["ppt/_rels/presentation.xml.rels"] = xmlHeader + rels + `</Relationships>`
	entries["ppt/slideMasters/slideMaster1.xml"] = xmlHeader + `<p:sldMaster ` + pns + `><p:cSld><p:spTree>` + group + `</p:spTree></p:cSld><p:clrMap accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" bg1="lt1" bg2="lt2" folHlink="folHlink" hlink="hlink" tx1="dk1" tx2="dk2"/><p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rId1"/></p:sldLayoutIdLst><p:txStyles><p:titleStyle/><p:bodyStyle/><p:otherStyle/></p:txStyles></p:sldMaster>`
	entries["ppt/slideMasters/_rels/slideMaster1.xml.rels"] = xmlHeader + `<Relationships xmlns="` + relNS + `"><Relationship Id="rId1" Type="` + officeNS + `/slideLayout" Target="../slideLayouts/slideLayout1.xml"/><Relationship Id="rId2" Type="` + officeNS + `/theme" Target="../theme/theme1.xml"/></Relationships>`
	entries["ppt/slideLayouts/slideLayout1.xml"] = xmlHeader + `<p:sldLayout ` + pns + ` type="blank" preserve="1"><p:cSld name="Blank"><p:spTree>` + group + `</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sldLayout>`
	entries["ppt/slideLayouts/_rels/slideLayout1.xml.rels"] = xmlHeader + `<Relationships xmlns="` + relNS + `"><Relationship Id="rId1" Type="` + officeNS + `/slideMaster" Target="../slideMasters/slideMaster1.xml"/></Relationships>`
	colors := ""
	for i, n := range []string{"dk1", "lt1", "dk2", "lt2", "accent1", "accent2", "accent3", "accent4", "accent5", "accent6", "hlink", "folHlink"} {
		c := []string{"172B4D", "FFFFFF", "334155", "F5F7FB", "3765D9", "17A398", "F0AD4E", "926CD2", "DD6E63", "5E8FB5", "3765D9", "926CD2"}[i]
		colors += "<a:" + n + "><a:srgbClr val=\"" + c + "\"/></a:" + n + ">"
	}
	fill := `<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>`
	line := `<a:ln w="9525">` + fill + `<a:prstDash val="solid"/></a:ln>`
	entries["ppt/theme/theme1.xml"] = xmlHeader + `<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="OpsBuddy"><a:themeElements><a:clrScheme name="OpsBuddy">` + colors + `</a:clrScheme><a:fontScheme name="OpsBuddy"><a:majorFont><a:latin typeface="Calibri"/><a:ea typeface="Microsoft YaHei"/><a:cs typeface="Arial"/></a:majorFont><a:minorFont><a:latin typeface="Calibri"/><a:ea typeface="Microsoft YaHei"/><a:cs typeface="Arial"/></a:minorFont></a:fontScheme><a:fmtScheme name="OpsBuddy"><a:fillStyleLst>` + fill + fill + fill + `</a:fillStyleLst><a:lnStyleLst>` + line + line + line + `</a:lnStyleLst><a:effectStyleLst><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle></a:effectStyleLst><a:bgFillStyleLst>` + fill + fill + fill + `</a:bgFillStyleLst></a:fmtScheme></a:themeElements></a:theme>`
	return writeZip(path, entries)
}
func xlsxFile(path string, d Deliverable) error {
	if len(d.Headers) == 0 {
		return errors.New("表格需要至少一列表头")
	}
	f := excelize.NewFile()
	defer f.Close()
	sheet := "Sheet1"
	for col, h := range d.Headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		if e := f.SetCellStr(sheet, cell, h); e != nil {
			return e
		}
	}
	for row, values := range d.Rows {
		for col, v := range values {
			if col >= len(d.Headers) {
				break
			}
			cell, _ := excelize.CoordinatesToCellName(col+1, row+2)
			var e error
			switch x := v.(type) {
			case nil:
				e = f.SetCellStr(sheet, cell, "")
			case float64:
				e = f.SetCellFloat(sheet, cell, x, -1, 64)
			case int:
				e = f.SetCellInt(sheet, cell, int64(x))
			default:
				e = f.SetCellStr(sheet, cell, fmt.Sprint(v))
			}
			if e != nil {
				return e
			}
		}
	}
	style, e := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"3765D9"}, Pattern: 1}})
	if e != nil {
		return e
	}
	last, _ := excelize.ColumnNumberToName(len(d.Headers))
	_ = f.SetCellStyle(sheet, "A1", last+"1", style)
	_ = f.SetColWidth(sheet, "A", last, 22)
	_ = f.SetRowHeight(sheet, 1, 28)
	_ = f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2"})
	_ = f.AutoFilter(sheet, "A1:"+last+strconv.Itoa(len(d.Rows)+1), nil)
	return f.SaveAs(path)
}
func readDocument(path, kind string) (string, error) {
	switch kind {
	case "txt", "md", "html", "css", "js", "mjs", "cjs", "go", "mod", "sum", "py", "csv", "tsv", "json":
		b, e := os.ReadFile(path)
		if e != nil {
			return "", e
		}
		if !utf8.Valid(b) {
			return "", errors.New("文本不是 UTF-8 编码，请转换后重试")
		}
		return string(b), nil
	case "docx", "pptx":
		z, e := zip.OpenReader(path)
		if e != nil {
			return "", e
		}
		defer z.Close()
		var out strings.Builder
		total := int64(0)
		for _, f := range z.File {
			selected := f.Name == "word/document.xml" || (strings.HasPrefix(f.Name, "ppt/slides/slide") && strings.HasSuffix(f.Name, ".xml"))
			if !selected {
				continue
			}
			if f.UncompressedSize64 > 8<<20 {
				return "", errors.New("文档解压后过大")
			}
			total += int64(f.UncompressedSize64)
			if total > 32<<20 {
				return "", errors.New("文档内容过大")
			}
			rc, e := f.Open()
			if e != nil {
				return "", e
			}
			dec := xml.NewDecoder(io.LimitReader(rc, 8<<20))
			text := false
			for {
				tok, e := dec.Token()
				if e == io.EOF {
					break
				}
				if e != nil {
					rc.Close()
					return "", e
				}
				switch t := tok.(type) {
				case xml.StartElement:
					if t.Name.Local == "t" {
						text = true
					}
				case xml.EndElement:
					if t.Name.Local == "t" {
						text = false
					}
					if t.Name.Local == "p" {
						out.WriteString("\n")
					}
				case xml.CharData:
					if text {
						out.Write(t)
					}
				}
			}
			rc.Close()
		}
		return out.String(), nil
	case "xlsx":
		f, e := excelize.OpenFile(path, excelize.Options{UnzipSizeLimit: 64 << 20})
		if e != nil {
			return "", e
		}
		defer f.Close()
		var out strings.Builder
		for _, sheet := range f.GetSheetList() {
			rows, e := f.GetRows(sheet)
			if e != nil {
				return "", e
			}
			if len(rows) > 10000 {
				return "", errors.New("本版预览/模型输入最多支持每表 10000 行")
			}
			out.WriteString("工作表：" + sheet + "\n")
			w := csv.NewWriter(&out)
			for _, row := range rows {
				_ = w.Write(row)
			}
			w.Flush()
		}
		return out.String(), nil
	case "pdf":
		return pdfText(path)
	default:
		return "", fmt.Errorf("暂不支持读取 .%s；可使用文件分类打包", kind)
	}
}
func pdfText(path string) (text string, err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("PDF 文本解析失败，文件可能包含不支持的编码")
		}
	}()
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return "", e
	}
	reader, e := pdf.NewReader(f, info.Size())
	if e != nil {
		return "", e
	}
	if reader.NumPage() > 200 {
		return "", errors.New("本版文本提取最多支持 200 页")
	}
	var out strings.Builder
	for n := 1; n <= reader.NumPage(); n++ {
		content := reader.Page(n).Content()
		out.WriteString(fmt.Sprintf("\n[第 %d 页]\n", n))
		if len(content.Text) == 0 {
			out.WriteString("[未检测到文本层；可能需要 OCR，本版暂不支持]\n")
		}
		sort.Sort(pdf.TextVertical(content.Text))
		var previous pdf.Text
		for i, t := range content.Text {
			if i > 0 {
				if math.Abs(t.Y-previous.Y) > 2 {
					out.WriteString("\n")
				} else if t.X-(previous.X+previous.W) > math.Max(1, t.FontSize*0.16) {
					out.WriteString(" ")
				}
			}
			out.WriteString(t.S)
			previous = t
		}
	}
	return out.String(), nil
}
func pdfProcess(ctx context.Context, paths []string, out, action, pages string) error {
	pdfapi.DisableConfigDir()
	switch action {
	case "merge":
		if len(paths) < 2 {
			return errors.New("合并 PDF 请上传至少两个 PDF 文件")
		}
		return pdfapi.MergeCreateFile(ctx, paths, out, false, nil)
	case "split":
		if len(paths) != 1 {
			return errors.New("提取页面请只选择一个 PDF")
		}
		if pages == "" {
			return errors.New("请在需求中写明页码，例如：提取 2-4 页")
		}
		return pdfapi.TrimFile(ctx, paths[0], out, []string{pages}, nil)
	}
	return errors.New("未知 PDF 操作")
}
