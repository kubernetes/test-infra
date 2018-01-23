"use strict";

const closedArrow = "\u25b6 ";
const openedArrow = "\u25bc ";

function getParameterByName(name) {  // http://stackoverflow.com/a/5158301/3694
    var match = RegExp('[?&]' + name + '=([^&/]*)').exec(window.location.search);
    return match && decodeURIComponent(match[1].replace(/\+/g, ' '));
}

function redrawOptions() {
    var rs = allHelp.AllRepos.sort();
    var sel = document.getElementById("repo");
    while (sel.length > 1)
        sel.removeChild(sel.lastChild);
    var param = getParameterByName("repo");
    for (var i = 0; i < rs.length; i++) {
        var o = document.createElement("option");
        o.text = rs[i];
        if (param && rs[i] === param) {
            o.selected = true;
        }
        sel.appendChild(o);
    }
};

window.onload = function() {
    // set dropdown based on options from query string
    redrawOptions();
    redraw();
};

document.addEventListener("DOMContentLoaded", function(event) {
   configure();
});

function configure() {
    if(typeof branding === 'undefined'){
        return;
    }
    if (branding.logo) {
        document.getElementById('img').src = branding.logo;
    }
    if (branding.favicon) {
        document.getElementById('favicon').href = branding.favicon;
    }
    if (branding.background_color) {
        document.body.style.background = branding.background_color;
    }
    if (branding.header_color) {
        document.getElementsByTagName('header')[0].style.backgroundColor = branding.header_color;
    }
}

function selectionText(sel) {
    return sel.selectedIndex == 0 ? "" : sel.options[sel.selectedIndex].text;
}

// applicablePlugins takes an org/repo string and a repo to plugin map and returns the plugins that apply to the repo.
function applicablePlugins(repoSel, repoPlugins) {
    if (repoSel == "") {
        var all = repoPlugins[""];
        if (all) {
            return all.sort();
        }
        return [];
    }
    var parts = repoSel.split("/")
    var plugins = [];
    var byOrg = repoPlugins[parts[0]];
    if (byOrg && byOrg != []) {
        plugins = plugins.concat(byOrg);
    }
    var byRepo = repoPlugins[repoSel];
    if (byRepo) {
        for (var i = 0; i < byRepo.length; i ++) {
            if (!plugins.includes(byRepo[i])) {
                plugins.push(byRepo[i]);
            }
        }
    }
    return plugins.sort()
}

function addSection(div, section, elem) {
    var h4 = document.createElement("h4");
    h4.appendChild(document.createTextNode(section));
    h4.className = "plugin-section-header";
    div.appendChild(h4);
    elem.className = "indented";
    div.appendChild(elem);
}

function addTextSection(div, section, content) {
    if (!content) {
        return;
    }

    var p = document.createElement("p");
    p.innerHTML = content
    addSection(div, section, p);
}

function newPreElem(content) {
    var pre = document.createElement("pre");
    pre.appendChild(document.createTextNode(content));
    return pre;
}

function ulFromElemList(list) {
    var ul = document.createElement("ul");
    for (var i = 0; i < list.length; i++) {
        var li = document.createElement("li");
        li.appendChild(list[i]);
        ul.appendChild(li);
    }
    return ul;
}

function redrawHelpTable(repo, names, helpMap, tableParent) {
    var table = tableParent.getElementsByTagName("table")[0];
    if (!names || names.length == 0) {
        tableParent.style.display = "none";
        return
    } else {
        tableParent.style.display = "block";
    }

    var tbody = table.getElementsByTagName("tbody")[0];
    while (tbody.firstChild)
        tbody.removeChild(tbody.firstChild);

    for (var i = 0; i < names.length; i++) {
        var name = names[i];
        var help = helpMap[name];

        var div = document.createElement("div");
        div.style.display = "none";
        div.className = "plugin-description";
        if (help) {
            addTextSection(div, "Description", help.Description);
            addTextSection(div, "Who can use", help.WhoCanUse);
            if (help.Usage) {
                addSection(div, "Usage", newPreElem(help.Usage));
            }
            if (help.Examples && help.Examples != []) {
                addSection(div, "Examples", ulFromElemList(help.Examples.map(newPreElem)));
            }

            if (repo != "") {
                addTextSection(div, "Configuration (" + repo + ")", help.Config ? help.Config[repo] : "");
            }
            addTextSection(div, "Configuration (global)", help.Config ? help.Config[""] : "");
            var content = ""
            if (help.Events && help.Events != []) {
                content = "[" + help.Events.sort().join(", ") + "]"
            }
            addTextSection(div, "Events handled", content);
        } else {
            var p = document.createElement("p");
            p.appendChild(document.createTextNode("Failed to retrieve help information for this plugin."));
            div.appendChild(p);
        }

        var pluginHeader = document.createElement("div");
        pluginHeader.className = "plugin-header";
        pluginHeader.appendChild(document.createTextNode(closedArrow + name));
        pluginHeader.addEventListener("click", clickHandler(div), true);
        var outerDiv = document.createElement("div");
        outerDiv.appendChild(pluginHeader);
        outerDiv.appendChild(div);
        outerDiv.className = "plugin-help-row";
        var tr = document.createElement("tr");
        tr.appendChild(outerDiv);
        tr.id = "plugin-" + name;
        tbody.appendChild(tr);
    }
}

function clickHandler(div) {
    return function(event) {
        if (div.style.display == "none") {
            div.style.display = "block";
            event.target.textContent = openedArrow + event.target.textContent.slice(2);
        } else {
            div.style.display = "none";
            event.target.textContent = closedArrow + event.target.textContent.slice(2);
        }
    }
}

function redraw() {
    var normals = document.getElementById("normal-plugins");
    var externals = document.getElementById("external-plugins");

    var repoSel = selectionText(document.getElementById("repo"));
    if (window.history && window.history.replaceState !== undefined) {
        if (repoSel !== "") {
            history.replaceState(null, "", "/plugin-help.html?repo=" + encodeURIComponent(repoSel));
        } else {
            history.replaceState(null, "", "/plugin-help.html")
        }
    }
    redrawOptions();

    redrawHelpTable(
        repoSel,
        applicablePlugins(repoSel, allHelp.RepoPlugins),
        allHelp.PluginHelp,
        normals,
    );
    redrawHelpTable(
        repoSel,
        applicablePlugins(repoSel, allHelp.RepoExternalPlugins),
        allHelp.ExternalPluginHelp,
        externals,
    );
}
