/*
Copyright 2020 The Kubernetes Authors.

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

function addSectionExpanders(): void {
  const expanders = document.querySelectorAll<HTMLTableRowElement>('tr.section-expander');
  for (const expander of Array.from(expanders)) {
    expander.onclick = () => {
      const nextRow = expander.nextElementSibling!;
      const icon = expander.querySelector('i')!;
      if (nextRow.classList.contains('hidden-data')) {
        nextRow.classList.remove('hidden-data');
        icon.innerText = 'expand_less';
      } else {
        nextRow.classList.add('hidden-data');
        icon.innerText = 'expand_more';
      }
      spyglass.contentUpdated();
    };
  }
}

function resizeIframe(e: MessageEvent): void {
    const iFrame = document.getElementById(e.data.id) as HTMLIFrameElement;
    if (!iFrame) {
        return;
    }
    if (iFrame.contentWindow === e.source) {
        const height = e.data.height + "px";
        iFrame.height = height;
        iFrame.style.height = height;
        spyglass.contentUpdated();
    }
}

window.addEventListener('DOMContentLoaded', addSectionExpanders);
window.addEventListener('message', resizeIframe );
