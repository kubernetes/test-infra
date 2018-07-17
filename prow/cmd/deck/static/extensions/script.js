// This file serves for custom javascript extensions
function pageFromUri(page) {
    splitUri = page.split('/');
    page = splitUri[splitUri.length - 1];

    if (page === '') {
        page = 'index'
    }

    return page
}

function disableIncorrectNavbarLinks(pages) {
    var links = document.getElementsByClassName("mdl-navigation__link");
    disabledLinks = [];

    for (var i = 0; i < links.length; i++) {
        console.log(i + 'testing ' + links[i].href)
        if (!pages.includes(pageFromUri(links[i].href))) {
            disabledLinks.push(links[i])
        }
    }

    disabledLinks.forEach(function(elem) { elem.remove(); });
}

window.onload = function() {
    var req = new XMLHttpRequest();
    req.open("GET", "/enabled-pages");
    req.onreadystatechange = function() {
        if (this.readyState == 4 && this.status == 200) {
            let enabledPages = JSON.parse(req.responseText);

            if (!enabledPages.includes(pageFromUri(window.location.href))) {
                window.location.href = "/" + enabledPages[0];
            }

            disableIncorrectNavbarLinks(enabledPages);
        }
    };

    req.send();
};

