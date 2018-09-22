import "dialog-polyfill";

function getParameterByName(name) {  // http://stackoverflow.com/a/5158301/3694
    const match = new RegExp('[?&]' + name + '=([^&/]*)').exec(window.location.search);
    return match && decodeURIComponent(match[1].replace(/\+/g, ' '));
}

function redrawOptions() {
    const rs = allHelp.AllRepos.sort();
    const sel = document.getElementById("repo");
    while (sel.length > 1) {
        sel.removeChild(sel.lastChild);
    }
    const param = getParameterByName("repo");
    rs.forEach((opt) => {
        const o = document.createElement("option");
        o.text = opt;
        o.selected = (param && opt === param);
        sel.appendChild(o);
    });
}

window.onload = function () {
    // set dropdown based on options from query string
    const hash = window.location.hash;
    redrawOptions();
    redraw();

    // Register dialog
    const dialog = document.querySelector('dialog');
    dialogPolyfill.registerDialog(dialog);
    dialog.querySelector('.close').addEventListener('click', () => {
        dialog.close();
    });

    if (hash !== "") {
        const el = document.body.querySelector(hash);
        const mainContainer = document.body.querySelector(".mdl-layout__content");
        if (el && mainContainer) {
            setTimeout(() => {
                mainContainer.scrollTop = el.getBoundingClientRect().top;
                window.location.hash = hash;
            }, 32);
            el.querySelector(".mdl-button--primary").click();
        }
    }
};

document.addEventListener("DOMContentLoaded", () => {
    configure();
});

function configure() {
    if (!branding) {
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
    return sel.selectedIndex === 0 ? "" : sel.options[sel.selectedIndex].text;
}

/**
 * Takes an org/repo string and a repo to plugin map and returns the plugins
 * that apply to the repo.
 * @param {string} repoSel repo name
 * @param {Map<string, PluginHelp>} repoPlugins maps plugin name to plugin
 * @return {Array<string>}
 */
function applicablePlugins(repoSel, repoPlugins) {
    if (repoSel === "") {
        const all = repoPlugins[""];
        if (all) {
            return all.sort();
        }
        return [];
    }
    const parts = repoSel.split("/");
    const byOrg = repoPlugins[parts[0]];
    let plugins = [];
    if (byOrg && byOrg !== []) {
        plugins = plugins.concat(byOrg);
    }
    const pluginNames = repoPlugins[repoSel];
    if (pluginNames) {
        pluginNames.forEach((pluginName) => {
            if (!plugins.includes(pluginName)) {
                plugins.push(pluginName);
            }
        });
    }
    return plugins.sort();
}

/**
 * Returns a normal cell for the command row.
 * @param {Array<string>|string} data content of the cell
 * @param {Array<string>} styles a list of styles applied to the cell.
 * @param {boolean} noWrap true if the content of the cell should be wrap.
 * @return {Element}
 */
function createCommandCell(data, styles = [], noWrap = false) {
    const cell = document.createElement("TD");
    cell.classList.add("mdl-data-table__cell--non-numeric");
    if (!noWrap) {
        cell.classList.add("table-cell");
    }
    let content;
    if (Array.isArray((data))) {
        content = document.createElement("UL");
        content.classList = "command-example-list";

        data.forEach((item) => {
            const itemContainer = document.createElement("LI");
            const span = document.createElement("SPAN");
            span.innerHTML = item;
            span.classList.add(...styles);
            itemContainer.appendChild(span);
            content.appendChild(itemContainer);
        });
    } else {
        content = document.createElement("DIV");
        content.classList.add(...styles);
        content.innerHTML = data;
    }

    cell.appendChild(content);

    return cell;
}

/**
 * Returns an icon element.
 * @param {number} no no. command
 * @param {string} iconString icon name
 * @param {?Array<string>} styles list of styles of the icon
 * @param {string} tooltip tooltip string
 * @param {boolean} isButton true if icon is a button
 * @return {Element}
 */
function createIcon(no, iconString, styles = [], tooltip = "", isButton = false) {
    const icon = document.createElement("I");
    icon.id = "icon-" + iconString + "-" + no;
    icon.classList.add("material-icons");
    icon.classList.add(...styles);
    icon.innerHTML = iconString;

    const container = isButton ? document.createElement("BUTTON") : document.createElement("DIV");
    container.appendChild(icon);
    if (isButton) {
        container.classList.add(...["mdl-button", "mdl-js-button", "mdl-button--icon"]);
    }

    if (tooltip === "") {
        return container;
    }

    const tooltipEl = document.createElement("DIV");
    tooltipEl.setAttribute("for", icon.id);
    tooltipEl.classList.add("mdl-tooltip");
    tooltipEl.innerHTML = tooltip;
    container.appendChild(tooltipEl);

    return container;
}

/**
 * Returns the feature cell for the command row.
 * @param {boolean} isFeatured true if the command is featured.
 * @param {boolean} isExternal true if the command is external.
 * @param {number} no no. command.
 * @return {Element}
 */
function commandStatus(isFeatured, isExternal, no) {
    const status = document.createElement("TD");
    status.classList.add("mdl-data-table__cell--non-numeric");
    if (isFeatured) {
        status.appendChild(
            createIcon(no, "stars", ["featured-icon"], "Featured command"));
    }
    if (isExternal) {
        status.appendChild(
            createIcon(no, "open_in_new", ["external-icon"], "External plugin"));
    }
    return status;
}

/**
 * Returns a section to the content of the dialog
 * @param title title of the section
 * @param body body of the section
 * @return {Element}
 */
function addDialogSection(title, body) {
    const container = document.createElement("DIV");
    const sectionTitle = document.createElement("h5");
    const sectionBody = document.createElement("p");

    sectionBody.classList.add("dialog-section-body");
    sectionBody.innerHTML = body;

    sectionTitle.classList.add("dialog-section-title");
    sectionTitle.innerHTML = title;

    container.classList.add("dialog-section");
    container.appendChild(sectionTitle);
    container.appendChild(sectionBody);

    return container;
}

/**
 * Returns a cell for the Plugin column.
 * @param {string}repo repo name
 * @param {string} pluginName plugin name.
 * @param {PluginHelp} plugin the plugin to which the command belong to
 * @return {Element}
 */
function createPluginCell(repo, pluginName, plugin) {
    const pluginCell = document.createElement("TD");
    const button = document.createElement("button");
    pluginCell.classList.add("mdl-data-table__cell--non-numeric");
    button.classList.add("mdl-button", "mdl-button--js", "mdl-button--primary");
    button.innerHTML = pluginName;

    // Attach Event Handlers.
    const dialog = document.querySelector('dialog');
    button.addEventListener('click', (event) => {
        const title = dialog.querySelector(".mdl-dialog__title");
        const content = dialog.querySelector(".mdl-dialog__content");

        while (content.firstChild) {
            content.removeChild(content.firstChild);
        }

        title.innerHTML = pluginName;
        if (plugin.Description) {
            content.appendChild(addDialogSection("Description", plugin.Description));
        }
        if (plugin.Events) {
            const sectionContent = "[" + plugin.Events.sort().join(", ") + "]";
            content.appendChild(addDialogSection("Events handled", sectionContent));
        }
        if (plugin.Config) {
            let sectionContent = plugin.Config ? plugin.Config[repo] : "";
            let sectionTitle =
                repo === "" ? "Configuration(global)" : "Configuration(" + repo + ")";
            if (sectionContent && sectionContent !== "") {
                content.appendChild(addDialogSection(sectionTitle, sectionContent));
            }
        }
        dialog.showModal();
    });

    pluginCell.appendChild(button);
    return pluginCell;
}

/**
 * Creates a link that links to the command.
 * @param name
 * @param no
 * @return {Element}
 */
function createCommandLink(name, no) {
    const link = document.createElement("TD");
    const iconButton = createIcon(no, "link", ["link-icon"], "", true);

    iconButton.addEventListener("click", () => {
        const tempInput = document.createElement("INPUT");
        let url = window.location.href;
        const hashIndex = url.indexOf("#");
        if (hashIndex !== -1) {
            url = url.slice(0, hashIndex);
        }

        url += "#" + name;
        tempInput.style.zIndex = "-99999";
        tempInput.style.background = "transparent";
        tempInput.value = url;

        document.body.appendChild(tempInput);
        tempInput.select();
        document.execCommand("copy");
        document.body.removeChild(tempInput);

        const toast = document.body.querySelector("#toast");
        toast.MaterialSnackbar.showSnackbar({message: "Copied to clipboard"});
    });

    link.appendChild(iconButton);
    link.classList.add("mdl-data-table__cell--non-numeric");

    return link;
}


/**
 * Creates a row for the Command table.
 * @param {string} repo repo name.
 * @param {string} pluginName plugin name.
 * @param {PluginHelp} plugin the plugin to which the command belongs.
 * @param {Command} command the command.
 * @param {boolean} isExternal true if the command belongs to an external
 * @param {number} no no. command
 * @return {Element}
 */
function createCommandRow(repo, pluginName, plugin, command, isExternal, no) {
    const row = document.createElement("TR");
    const name = extractCommandName(command.Examples[0]);
    row.id = name;

    row.appendChild(commandStatus(command.Featured, isExternal, no));
    row.appendChild(createCommandCell(command.Usage, ["command-usage"]));
    row.appendChild(
        createCommandCell(command.Examples, ["command-examples"], true));
    row.appendChild(
        createCommandCell(command.Description, ["command-desc-text"]));
    row.appendChild(createCommandCell(command.WhoCanUse, ["command-desc-text"]));
    row.appendChild(createPluginCell(repo, pluginName, plugin));
    row.appendChild(createCommandLink(name, no));

    return row;
}

/**
 * Redraw a plugin table.
 * @param {string} repo repo name.
 * @param {Map<string, Object>} helpMap maps a plugin name to a plugin.
 */
function redrawHelpTable(repo, helpMap) {
    const table = document.getElementById("command-table");
    const tableBody = document.querySelector("TBODY");
    if (helpMap.size === 0) {
        table.style.display = "none";
        return;
    }
    table.style.display = "table";
    while (tableBody.childElementCount !== 0) {
        tableBody.removeChild(tableBody.firstChild);
    }
    const names = Array.from(helpMap.keys());
    const commandsWithPluginName = [];
    for (let name of names) {
        helpMap.get(name).plugin.Commands.forEach((command) => {
            commandsWithPluginName.push({
                pluginName: name,
                command: command
            });
        });
    }
    commandsWithPluginName
        .sort((command1, command2) => {
            return command1.command.Featured ? -1 : command2.command.Featured ? 1 : 0;
        })
        .forEach((command, index) => {
            const pluginName = command.pluginName;
            const {isExternal, plugin} = helpMap.get(pluginName);
            const commandRow = createCommandRow(
                repo,
                pluginName,
                plugin,
                command.command,
                isExternal,
                index);
            tableBody.appendChild(commandRow);
        });
}

/**
 * Redraws the content of the page.
 */
function redraw() {
    const repoSel = selectionText(document.getElementById("repo"));
    if (window.history && window.history.replaceState !== undefined) {
        if (repoSel !== "") {
            history.replaceState(null, "", "/command-help?repo="
                + encodeURIComponent(repoSel));
        } else {
            history.replaceState(null, "", "/command-help")
        }
    }
    redrawOptions();

    const pluginsWithCommands = new Map();
    applicablePlugins(repoSel, allHelp.RepoPlugins)
        .forEach((name) => {
            if (allHelp.PluginHelp[name] && allHelp.PluginHelp[name].Commands) {
                pluginsWithCommands.set(
                    name,
                    {
                        isExternal: false,
                        plugin: allHelp.PluginHelp[name]
                    });
            }
        });
    applicablePlugins(repoSel, allHelp.RepoExternalPlugins)
        .forEach((name) => {
            if (allHelp.ExternalPluginHelp[name]
                && allHelp.ExternalPluginHelp[name].Commands) {
                pluginsWithCommands.set(
                    name,
                    {
                        isExternal: true,
                        plugin: allHelp.ExternalPluginHelp[name]
                    });
            }
        });
    redrawHelpTable(repoSel, pluginsWithCommands);
}


/**
 * Extracts a command name from a command example. It takes the first example,
 * with out the slash, as the name for the command. Also, any '-' character is
 * replaced by '_' to make the name valid in the address.
 * @param {string} commandExample
 * @return {string}
 */
function extractCommandName(commandExample) {
    const command = commandExample.split(" ");
    if (!command || command.length === 0) {
        throw new Error("Cannot extract command name.");
    }
    return command[0].slice(1).split("-").join("_");
}

// This is referenced by name in the HTML.
window.redraw = redraw;
