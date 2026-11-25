/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"io"
	"strings"
	"text/template"

	prowv1 "sigs.k8s.io/prow/pkg/apis/prowjobs/v1"
	"sigs.k8s.io/prow/pkg/io/providers"
)

const tmplPageHeaderText = `
    <!doctype html>
   	<html>
   	<head>
   	    <link rel="stylesheet" type="text/css" href="/styles/style.css">
   	    <meta charset="utf-8">
   	    <meta name="viewport" content="width=device-width, initial-scale=1.0">
   	    <title>{{.ProviderName}} browser: {{.BucketName}}</title>
		<style>
		header {
			margin-left: 10px;
		}

		header h3 a,
		header h3 a:visited,
		header h3 a:hover,
		header h3 a:active {
			color: black;
			text-decoration: none;
		}

		header h3 a:hover {
			text-decoration: underline;
		}

		.next-button {
			margin: 10px 0;
		}

		.grid-head {
			border-bottom: 1px solid black;
		}

		.resource-grid {
			margin-right: 20px;
		}

		li.grid-row:nth-child(even) {
			background-color: #ddd;
		}

		li div {
			box-sizing: border-box;
			border-left: 1px solid black;
			padding-left: 5px;
			overflow-wrap: break-word;
		}
		li div:first-child {
			border-left: none;
		}

		.sort-toggle {
			background: none;
			border: none;
			padding: 0;
			font: inherit;
			color: inherit;
			cursor: pointer;
		}

		.sort-indicator {
			font-size: 0.85em;
			opacity: 0.6;
		}

		.sort-toggle:hover .sort-indicator,
		.sort-toggle[data-sort="asc"] .sort-indicator,
		.sort-toggle[data-sort="desc"] .sort-indicator {
			opacity: 1;
		}

		.copy-btn {
			background: none;
			border: none;
			padding: 0 2px;
			margin-left: 4px;
			cursor: pointer;
			color: #555;
			vertical-align: middle;
			line-height: 1;
			opacity: 0.7;
		}

		.copy-btn:hover {
			color: #000;
			opacity: 1;
		}

		.copy-btn.copied {
			color: #1a7f37;
			opacity: 1;
		}

		</style>
   	</head>
   	<body>
`

var tmplPageHeader = template.Must(template.New("page-header").Parse(tmplPageHeaderText))

func htmlPageHeader(out io.Writer, providerName, bucketName string) error {
	args := struct {
		ProviderName string
		BucketName   string
	}{
		ProviderName: providerName,
		BucketName:   bucketName,
	}
	return tmplPageHeader.Execute(out, args)
}

const tmplPageFooterText = `<svg style="display:none" aria-hidden="true">
	<symbol id="icon-copy" viewBox="0 0 16 16">
		<g fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
			<rect x="5.5" y="5.5" width="8" height="8" rx="1.2"></rect>
			<path d="M10.5 5.5V3.7A1.2 1.2 0 0 0 9.3 2.5H3.7A1.2 1.2 0 0 0 2.5 3.7v5.6a1.2 1.2 0 0 0 1.2 1.2h1.8"></path>
		</g>
	</symbol>
	<symbol id="icon-check" viewBox="0 0 16 16">
		<g fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
			<path d="M3 8.5l3.2 3.2L13 5"></path>
		</g>
	</symbol>
</svg>
<script>
(function() {
	var grid = document.querySelector('.resource-grid');
	if (!grid) return;

	var sortBtns = Array.prototype.slice.call(grid.querySelectorAll('.sort-toggle[data-sort-col]'));
	var allRows = Array.prototype.slice.call(grid.querySelectorAll('li.grid-row'));
	var dirEntries = [];
	var fileEntries = [];
	for (var i = 0; i < allRows.length; i++) {
		var row = allRows[i];
		var link = row.querySelector('a');
		var name = link ? link.textContent.trim() : '';
		if (row.hasAttribute('data-mtime')) {
			fileEntries.push({
				row: row,
				name: name,
				size: Number(row.getAttribute('data-size')),
				mtime: Number(row.getAttribute('data-mtime')),
				index: fileEntries.length
			});
		} else if (row.querySelector('.copy-btn')) {
			dirEntries.push({
				row: row,
				name: name,
				index: dirEntries.length
			});
		}
	}
	var lastRow = fileEntries.length
		? fileEntries[fileEntries.length - 1].row
		: (dirEntries.length ? dirEntries[dirEntries.length - 1].row : null);
	var afterItemsNode = lastRow ? lastRow.nextSibling : null;

	function compareStrings(a, b) {
		return a < b ? -1 : (a > b ? 1 : 0);
	}

	sortBtns.forEach(function(btn) {
		btn.addEventListener('click', function() {
			var col = btn.getAttribute('data-sort-col');
			var current = btn.getAttribute('data-sort') || 'none';
			var next = current === 'none' ? 'desc' : (current === 'desc' ? 'asc' : 'none');

			sortBtns.forEach(function(other) {
				var active = other === btn ? next : 'none';
				other.setAttribute('data-sort', active);
				var ind = other.querySelector('.sort-indicator');
				if (ind) {
					ind.textContent = active === 'desc' ? '↓' : (active === 'asc' ? '↑' : '⇅');
				}
			});

			if (!dirEntries.length && !fileEntries.length) return;

			var sortedDirs = dirEntries.slice().sort(function(a, b) {
				if (next === 'none' || col !== 'name') return a.index - b.index;
				var cmp = compareStrings(a.name, b.name);
				if (cmp !== 0) return next === 'asc' ? cmp : -cmp;
				return a.index - b.index;
			});

			var sortedFiles = fileEntries.slice().sort(function(a, b) {
				if (next === 'none') return a.index - b.index;
				var diff = 0;
				if (col === 'name') {
					diff = compareStrings(a.name, b.name);
				} else if (col === 'size') {
					diff = a.size - b.size;
				} else if (col === 'modified') {
					diff = a.mtime - b.mtime;
				}
				if (diff !== 0) return next === 'asc' ? diff : -diff;
				return a.index - b.index;
			});

			var frag = document.createDocumentFragment();
			for (var d = 0; d < sortedDirs.length; d++) {
				frag.appendChild(sortedDirs[d].row);
			}
			for (var f = 0; f < sortedFiles.length; f++) {
				frag.appendChild(sortedFiles[f].row);
			}
			grid.insertBefore(frag, afterItemsNode);
		});
	});

	function fallbackCopyText(text) {
		return new Promise(function(resolve, reject) {
			var ta = document.createElement('textarea');
			ta.value = text;
			ta.style.position = 'fixed';
			ta.style.opacity = '0';
			document.body.appendChild(ta);
			ta.focus();
			ta.select();
			try {
				if (document.execCommand('copy')) {
					resolve();
				} else {
					reject(new Error('copy failed'));
				}
			} catch (err) {
				reject(err);
			} finally {
				document.body.removeChild(ta);
			}
		});
	}

	function copyText(text) {
		if (navigator.clipboard && navigator.clipboard.writeText) {
			return navigator.clipboard.writeText(text).catch(function() {
				return fallbackCopyText(text);
			});
		}
		return fallbackCopyText(text);
	}

	grid.addEventListener('click', function(e) {
		var btn = e.target.closest ? e.target.closest('.copy-btn') : null;
		if (!btn) return;
		var link = btn.parentElement && btn.parentElement.querySelector('a');
		if (!link) return;
		copyText(link.href).then(function() {
			var useEl = btn.querySelector('use');
			if (useEl) useEl.setAttribute('href', '#icon-check');
			btn.classList.add('copied');
			btn.setAttribute('title', 'Copied!');
			clearTimeout(btn._copyTimer);
			btn._copyTimer = setTimeout(function() {
				if (useEl) useEl.setAttribute('href', '#icon-copy');
				btn.classList.remove('copied');
				btn.setAttribute('title', 'Copy link');
			}, 1500);
		});
	});
})();
</script>
</body></html>`

var tmplPageFooter = template.Must(template.New("page-footer").Parse(tmplPageFooterText))

func htmlPageFooter(out io.Writer) error {
	return tmplPageFooter.Execute(out, struct{}{})
}

type breadcrumb struct {
	Name string
	URL  string
}

func buildBreadcrumbs(provPrefix, displayPath string) []breadcrumb {
	trimmed := strings.Trim(displayPath, "/")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, "/")
	crumbs := make([]breadcrumb, 0, len(parts))
	curr := strings.TrimSuffix(provPrefix, "/")
	for _, part := range parts {
		if part == "" {
			continue
		}
		curr += "/" + part
		crumbs = append(crumbs, breadcrumb{
			Name: part,
			URL:  curr + "/",
		})
	}
	return crumbs
}

const tmplContentHeaderText = `
    <header>
        <h1>{{.BucketName}}</h1>
        <h3>/{{range .Breadcrumbs}}<a href="{{.URL}}">{{.Name}}</a>/{{end}}</h3>
    </header>
    <ul class="resource-grid">
`

var tmplContentHeader = template.Must(template.New("content-header").Parse(tmplContentHeaderText))

func htmlContentHeader(out io.Writer, bucketName, provPrefix, path string) error {
	args := struct {
		BucketName  string
		Breadcrumbs []breadcrumb
	}{
		BucketName:  bucketName,
		Breadcrumbs: buildBreadcrumbs(provPrefix, path),
	}
	return tmplContentHeader.Execute(out, args)
}

const tmplContentFooterText = `</ul>
{{- if eq .ProwPath.StorageProvider .GSProvider }}
<details>
	<summary style="display: list-item; padding-left: 1em">Download</summary>
	<div style="padding: 1em">
		You can download this directory by running the following <a href="https://cloud.google.com/sdk/docs/install">gcloud storage</a> command:
		<pre>gcloud storage cp -r {{.ProwPath.StorageProvider}}://{{.ProwPath.FullPath}}/ .</pre>
	</div>
</details>
{{- end }}
`

var tmplContentFooter = template.Must(template.New("content-footer").Parse(tmplContentFooterText))

func htmlContentFooter(out io.Writer, prowPath *prowv1.ProwPath) error {
	args := struct {
		ProwPath   *prowv1.ProwPath
		GSProvider string
	}{
		ProwPath:   prowPath,
		GSProvider: providers.GS,
	}
	return tmplContentFooter.Execute(out, args)
}

const tmplNextButtonText = `
    <a href="{{.Path}}?marker={{.Marker}}"
	   class="pure-button next-button">
	   Next page
	</a>
`

var tmplNextButton = template.Must(template.New("next-button").Parse(tmplNextButtonText))

func htmlNextButton(out io.Writer, path, marker string) error {
	args := struct {
		Path   string
		Marker string
	}{
		Path:   path,
		Marker: marker,
	}
	return tmplNextButton.Execute(out, args)
}

const tmplGridHeaderText = `
	<li class="pure-g">
		<div class="pure-u-2-5 grid-head"><button type="button" class="sort-toggle" data-sort-col="name" title="Sort by Name">Name <span class="sort-indicator" aria-hidden="true">⇅</span></button></div>
		<div class="pure-u-1-5 grid-head"><button type="button" class="sort-toggle" data-sort-col="size" title="Sort by Size">Size <span class="sort-indicator" aria-hidden="true">⇅</span></button></div>
		<div class="pure-u-2-5 grid-head"><button type="button" class="sort-toggle" data-sort-col="modified" title="Sort by Modified">Modified <span class="sort-indicator" aria-hidden="true">⇅</span></button></div>
	</li>
`

var tmplGridHeader = template.Must(template.New("grid-header").Parse(tmplGridHeaderText))

func htmlGridHeader(out io.Writer) error {
	return tmplGridHeader.Execute(out, struct{}{})
}

type gridItem struct {
	URL      string
	Icon     string
	Name     string
	Size     string
	RawSize  int64
	Modified string
	MTime    int64
	IsFile   bool
	CanCopy  bool
}

const tmplGridItemText = `
    <li class="pure-g grid-row"{{if .IsFile}} data-size="{{.RawSize}}" data-mtime="{{.MTime}}"{{end}}>
	    <div class="pure-u-2-5"><a href="{{.URL}}"><img src="{{.Icon}}"> {{.Name}}</a>{{if .CanCopy}}<button type="button" class="copy-btn" title="Copy link" aria-label="Copy link"><svg width="12" height="12" viewBox="0 0 16 16" aria-hidden="true"><use href="#icon-copy"></use></svg></button>{{end}}</div>
	    <div class="pure-u-1-5">{{.Size}}</div>
	    <div class="pure-u-2-5">{{.Modified}}</div>
	</li>
`

var tmplGridItem = template.Must(template.New("grid-item").Parse(tmplGridItemText))

func htmlGridItem(out io.Writer, item gridItem) error {
	return tmplGridItem.Execute(out, item)
}
