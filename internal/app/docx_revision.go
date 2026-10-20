package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

type docParagraph struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	start, end int
}

func docxParagraphs(raw []byte) ([]docParagraph, error) {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	out := []docParagraph{}
	tableDepth := 0
	start := -1
	textDepth := 0
	var text strings.Builder
	for {
		offset := int(decoder.InputOffset())
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch v := token.(type) {
		case xml.StartElement:
			if v.Name.Local == "tbl" {
				tableDepth++
			}
			if v.Name.Local == "p" && tableDepth == 0 {
				start = offset
				text.Reset()
			}
			if v.Name.Local == "t" {
				textDepth++
			}
		case xml.CharData:
			if start >= 0 && textDepth > 0 {
				text.Write(v)
			}
		case xml.EndElement:
			if v.Name.Local == "t" {
				textDepth--
			}
			if v.Name.Local == "p" && tableDepth == 0 && start >= 0 {
				out = append(out, docParagraph{ID: fmt.Sprintf("p%d", len(out)), Text: text.String(), start: start, end: int(decoder.InputOffset())})
				start = -1
			}
			if v.Name.Local == "tbl" {
				tableDepth--
			}
		}
	}
	return out, nil
}
func readDocxBody(path string) ([]byte, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	for _, f := range z.File {
		if f.Name == "word/document.xml" {
			r, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer r.Close()
			raw, err := io.ReadAll(io.LimitReader(r, 8<<20+1))
			if len(raw) > 8<<20 {
				return nil, errors.New("文档正文过大，不能局部修改")
			}
			return raw, err
		}
	}
	return nil, errors.New("DOCX 缺少正文")
}
func revisionCatalog(path string) (string, error) {
	raw, err := readDocxBody(path)
	if err != nil {
		return "", err
	}
	paras, err := docxParagraphs(raw)
	if err != nil {
		return "", err
	}
	if len(paras) > 1500 {
		return "", errors.New("局部修改最多支持 1500 个正文段落，请拆分文档")
	}
	catalog, _ := json.Marshal(paras)
	return string(catalog), nil
}

var paragraphText = regexp.MustCompile(`(?s)<(?:[A-Za-z_][\w.-]*:)?t(?:\s[^>]*)?>.*?</(?:[A-Za-z_][\w.-]*:)?t>`)

func reviseDocxFile(destination, source string, d Deliverable) error {
	raw, err := readDocxBody(source)
	if err != nil {
		return err
	}
	paras, err := docxParagraphs(raw)
	if err != nil {
		return err
	}
	changes := map[string]string{}
	for _, row := range d.Rows {
		if len(row) != 2 {
			return errors.New("模型的段落修改格式无效")
		}
		key, ok := row[0].(string)
		value, valid := row[1].(string)
		if !ok || !valid || len(value) > 30000 {
			return errors.New("模型的段落替换内容无效")
		}
		if _, exists := changes[key]; exists {
			return errors.New("模型重复修改同一段落")
		}
		changes[key] = value
	}
	var output bytes.Buffer
	cursor := 0
	for _, p := range paras {
		value, ok := changes[p.ID]
		if !ok {
			continue
		}
		fragment := raw[p.start:p.end]
		matches := paragraphText.FindAllIndex(fragment, -1)
		if len(matches) == 0 {
			return errors.New("所选段落不含可安全替换的文本")
		}
		output.Write(raw[cursor:p.start])
		last := 0
		for i, bounds := range matches {
			output.Write(fragment[last:bounds[0]])
			tag := fragment[bounds[0]:bounds[1]]
			opening := bytes.IndexByte(tag, '>')
			closing := bytes.LastIndex(tag, []byte("</"))
			output.Write(tag[:opening+1])
			if i == 0 {
				_ = xml.EscapeText(&output, []byte(value))
			}
			output.Write(tag[closing:])
			last = bounds[1]
		}
		output.Write(fragment[last:])
		cursor = p.end
		delete(changes, p.ID)
	}
	if len(changes) > 0 {
		return errors.New("模型引用了不存在的正文段落")
	}
	output.Write(raw[cursor:])
	z, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer z.Close()
	file, err := os.Create(destination)
	if err != nil {
		return err
	}
	writer := zip.NewWriter(file)
	for _, entry := range z.File {
		if entry.UncompressedSize64 > 32<<20 {
			err = errors.New("文档包含过大的嵌入内容，不能安全局部修改")
			break
		}
		header := entry.FileHeader
		target, e := writer.CreateHeader(&header)
		if e != nil {
			err = e
			break
		}
		if entry.Name == "word/document.xml" {
			_, err = target.Write(output.Bytes())
		} else {
			input, e := entry.Open()
			if e != nil {
				err = e
				break
			}
			_, err = io.Copy(target, input)
			input.Close()
		}
		if err != nil {
			break
		}
	}
	closeZip := writer.Close()
	closeFile := file.Close()
	if err != nil {
		return err
	}
	if closeZip != nil {
		return closeZip
	}
	return closeFile
}
