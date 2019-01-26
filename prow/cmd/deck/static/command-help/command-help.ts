import "dialog-polyfill";
import {Command, Help, PluginHelp} from "../api/help";

declare const allHelp: Help;

declare const dialogPolyfill: {
    registerDialog(element: HTMLDialogElement): void;
};

function getParameterByName(name: string): string | null {  // http://stackoverflow.com/a/5158301/3694
    const match = new RegExp('[?&]' + name + '=([^&/]*)').exec(window.location.search);
    return match && decodeURIComponent(match[1].replace(/\+/g, ' '));
}

function redrawOptions(): void {
    const rs = allHelp.AllRepos.sort();
    const sel = document.getElementById("repo") as HTMLSelectElement;
    while (sel.length > 1) {
        sel.removeChild(sel.lastChild!);
    }
    const param = getParameterByName("repo");
    rs.forEach((opt) => {
        const o = document.createElement("option");
        o.text = opt;
        o.selected = !!(param && opt === param);
        sel.appendChild(o);
    });
}

window.onload = function (): void {
    // set dropdown based on options from query string
    const hash = window.location.hash;
    redrawOptions();
    redraw();

    // Register dialog
    const dialog = document.querySelector('dialog') as HTMLDialogElement;
    dialogPolyfill.registerDialog(dialog);
    dialog.querySelector('.close')!.addEventListener('click', () => {
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
            (el.querySelector(".mdl-button--primary") as HTMLButtonElement).click();
        }
    }
};

function selectionText(sel: HTMLSelectElement): string {
    return sel.selectedIndex === 0 ? "" : sel.options[sel.selectedIndex].text;
}

/**
 * Takes an org/repo string and a repo to plugin map and returns the plugins
 * that apply to the repo.
 * @param repoSel repo name
 * @param repoPlugins maps plugin name to plugin
 */
function applicablePlugins(repoSel: string, repoPlugins: {[key: string]: string[]}): string[] {
    if (repoSel === "") {
        const all = repoPlugins[""];
        if (all) {
            return all.sort();
        }
        return [];
    }
    const parts = repoSel.split("/");
    const byOrg = repoPlugins[parts[0]];
    let plugins: string[] = [];
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
 * @param data content of the cell
 * @param styles a list of styles applied to the cell.
 * @param noWrap true if the content of the cell should be wrap.
 */
function createCommandCell(data: string | string[], styles: string[] = [], noWrap = false): HTMLTableDataCellElement {
    const cell = document.createElement("td");
    cell.classList.add("mdl-data-table__cell--non-numeric");
    if (!noWrap) {
        cell.classList.add("table-cell");
    }
    let content: HTMLElement;
    if (Array.isArray(data)) {
        content = document.createElement("ul");
        content.classList.add("command-example-list");

        data.forEach((item) => {
            const itemContainer = document.createElement("li");
            const span = document.createElement("span");
            span.innerHTML = item;
            span.classList.add(...styles);
            itemContainer.appendChild(span);
            content.appendChild(itemContainer);
        });
    } else {
        content = document.createElement("div");
        content.classList.add(...styles);
        content.innerHTML = data;
    }

    cell.appendChild(content);

    return cell;
}

/**
 * Returns an icon element.
 * @param no no. command
 * @param iconString icon name
 * @param styles list of styles of the icon
 * @param tooltip tooltip string
 * @param isButton true if icon is a button
 */
function createIcon(no: number, iconString: string, styles: string[], tooltip: string, isButton?: false): HTMLDivElement
function createIcon(no: number, iconString: string, styles: string[], tooltip: string, isButton?: true): HTMLButtonElement
function createIcon(no: number, iconString: string, styles: string[] = [], tooltip: string = "", isButton = false) {
    const icon = document.createElement("i");
    icon.id = "icon-" + iconString + "-" + no;
    icon.classList.add("material-icons");
    icon.classList.add(...styles);
    icon.innerHTML = iconString;

    const container = isButton ? document.createElement("button") : document.createElement("div");
    container.appendChild(icon);
    if (isButton) {
        container.classList.add(...["mdl-button", "mdl-js-button", "mdl-button--icon"]);
    }

    if (tooltip === "") {
        return container;
    }

    const tooltipEl = document.createElement("div");
    tooltipEl.setAttribute("for", icon.id);
    tooltipEl.classList.add("mdl-tooltip");
    tooltipEl.innerHTML = tooltip;
    container.appendChild(tooltipEl);

    return container;
}

/**
 * Returns the feature cell for the command row.
 * @param isFeatured true if the command is featured.
 * @param isExternal true if the command is external.
 * @param no no. command.
 */
function commandStatus(isFeatured: boolean, isExternal: boolean, no: number): HTMLTableDataCellElement {
    const status = document.createElement("td");
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
 */
function addDialogSection(title: string, body: string): HTMLElement {
    const container = document.createElement("div");
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
 * @param repo repo name
 * @param pluginName plugin name.
 * @param plugin the plugin to which the command belong to
 */
function createPluginCell(repo: string, pluginName: string, plugin: PluginHelp): HTMLTableDataCellElement {
    const pluginCell = document.createElement("td");
    const button = document.createElement("button");
    pluginCell.classList.add("mdl-data-table__cell--non-numeric");
    button.classList.add("mdl-button", "mdl-button--js", "mdl-button--primary");
    button.innerHTML = pluginName;

    // Attach Event Handlers.
    const dialog = document.querySelector('dialog') as HTMLDialogElement;
    button.addEventListener('click', () => {
        const title = dialog.querySelector(".mdl-dialog__title")!;
        const content = dialog.querySelector(".mdl-dialog__content")!;

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
 */
function createCommandLink(name: string, no: number): HTMLTableDataCellElement {
    const link = document.createElement("td");
    const iconButton = createIcon(no, "link", ["link-icon"], "", true);

    iconButton.addEventListener("click", () => {
        const tempInput = document.createElement("input");
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

        const toast = document.body.querySelector("#toast")! as SnackbarElement;
        toast.MaterialSnackbar.showSnackbar({message: "Copied to clipboard"});
    });

    link.appendChild(iconButton);
    link.classList.add("mdl-data-table__cell--non-numeric");

    return link;
}


/**
 * Creates a row for the Command table.
 * @param repo repo name.
 * @param pluginName plugin name.
 * @param plugin the plugin to which the command belongs.
 * @param command the command.
 * @param isExternal true if the command belongs to an external
 * @param no no. command
 */
function createCommandRow(repo: string, pluginName: string, plugin: PluginHelp, command: Command, isExternal: boolean, no: number): HTMLTableRowElement {
    const row = document.createElement("tr");
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
 * @param repo repo name.
 * @param helpMap maps a plugin name to a plugin.
 */
function redrawHelpTable(repo: string, helpMap: Map<string, {isExternal: boolean, plugin: PluginHelp}>): void {
    const table = document.getElementById("command-table")!;
    const tableBody = document.querySelector("tbody")!;
    if (helpMap.size === 0) {
        table.style.display = "none";
        return;
    }
    table.style.display = "table";
    while (tableBody.childElementCount !== 0) {
        tableBody.removeChild(tableBody.firstChild!);
    }
    const names = Array.from(helpMap.keys());
    const commandsWithPluginName: {pluginName: string, command: Command}[] = [];
    for (let name of names) {
        helpMap.get(name)!.plugin.Commands.forEach((command) => {
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
            const {isExternal, plugin} = helpMap.get(pluginName)!;
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
function redraw(): void {
    const repoSel = selectionText(document.getElementById("repo") as HTMLSelectElement);
    if (window.history && window.history.replaceState !== undefined) {
        if (repoSel !== "") {
            history.replaceState(null, "", "/command-help?repo="
                + encodeURIComponent(repoSel));
        } else {
            history.replaceState(null, "", "/command-help")
        }
    }
    redrawOptions();

    const pluginsWithCommands: Map<string, {isExternal: boolean, plugin: PluginHelp}> = new Map();
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
 */
function extractCommandName(commandExample: string): string {
    const command = commandExample.split(" ");
    if (!command || command.length === 0) {
        throw new Error("Cannot extract command name.");
    }
    return command[0].slice(1).split("-").join("_");
}

// This is referenced by name in the HTML.
(window as any)['redraw'] = redraw;
