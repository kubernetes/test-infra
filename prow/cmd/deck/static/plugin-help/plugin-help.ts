import "dialog-polyfill";
import {Command, Help, PluginHelp} from "../api/help";

declare const dialogPolyfill: {
  registerDialog(element: HTMLDialogElement): void;
};

declare const allHelp: Help;

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
    redrawOptions();
    redraw();
    // Register dialog
    const dialog = document.querySelector('dialog') as HTMLDialogElement;
    dialogPolyfill.registerDialog(dialog);
    dialog.querySelector('.close')!.addEventListener('click', () => {
        dialog.close();
    });
    // Set up dropdown
    document.querySelector<HTMLSelectElement>('#repo')!.onchange = redraw;
};

function selectionText(sel: HTMLSelectElement): string {
    return sel.selectedIndex === 0 ? "" : sel.options[sel.selectedIndex].text;
}

/**
 * Returns a section to the content of the dialog
 * @param title title of the section
 * @param body body of the section
 */
function addDialogSection(title: string, body: string | HTMLElement[]): HTMLElement {
    const container = document.createElement("div");
    const sectionTitle = document.createElement("h5");
    const sectionBody = document.createElement("div");

    sectionBody.classList.add("dialog-section-body");
    if (Array.isArray(body)) {
        body.forEach(el => {
           sectionBody.appendChild(el);
        });
    } else {
        sectionBody.innerHTML = body;
    }

    sectionTitle.classList.add("dialog-section-title");
    sectionTitle.innerHTML = title;

    container.classList.add("dialog-section");
    container.appendChild(sectionTitle);
    container.appendChild(sectionBody);

    return container;
}

/**
 * Return a list of link elements that links to commands.
 * @param commands list of commands
 */
function getLinkableCommands(commands: Command[]): HTMLAnchorElement[] {
    const result: HTMLAnchorElement[] = [];
    commands.forEach(command => {
       const commandName = extractCommandName(command.Examples[0]);
       const link = document.createElement("a");
       link.href = "/command-help#" + commandName;
       link.innerHTML = command.Examples[0];
       link.classList.add("plugin-help-command-link");
       result.push(link);
    });
    return result;
}

/**
 * Create a card for a plugin.
 * @param repo repo name
 * @param name name of the plugin
 * @param pluginObj plugin object
 * @return the card element that contains the plugin
 */
function createPlugin(repo: string, name: string, pluginObj: {isExternal: boolean, plugin: PluginHelp}): HTMLElement {
    const isExternal = pluginObj.isExternal;
    const plugin = pluginObj.plugin;

    const title = document.createElement("h3");
    title.innerHTML = name;
    title.classList.add("mdl-card__title-text");
    const supportTitle = document.createElement("div");
    supportTitle.innerHTML = isExternal ? " external plugin" : "";
    supportTitle.classList.add("mdl-card__subtitle-text");
    const cardTitle = document.createElement("div");
    cardTitle.classList.add("mdl-card__title");
    cardTitle.appendChild(title);
    cardTitle.appendChild(supportTitle);

    const cardDesc = document.createElement("div");
    cardDesc.innerHTML = getFirstSentence(plugin.Description);
    cardDesc.classList.add("mdl-card__supporting-text");

    const cardAction = document.createElement("div");
    const actionButton = document.createElement("a");
    actionButton.innerHTML = "Details";
    actionButton.classList.add(...["mdl-button", "mdl-button--colored", "mdl-js-button", "mdl-js-ripple-effect"]);
    actionButton.addEventListener("click", () => {
        const dialog = document.querySelector("dialog") as HTMLDialogElement;
        const title = dialog.querySelector(".mdl-dialog__title")!;
        const content = dialog.querySelector(".mdl-dialog__content")!;

        while (content.firstChild) {
            content.removeChild(content.firstChild);
        }

        title.innerHTML = name;
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
        if (plugin.Commands) {
            let sectionContent = getLinkableCommands(plugin.Commands);
            content.appendChild(addDialogSection("Commands", sectionContent));
        }
        dialog.showModal();
    });
    cardAction.appendChild(actionButton);
    cardAction.classList.add("mdl-card__actions", "mdl-card--border");

    const card = document.createElement("div");
    card.appendChild(cardTitle);
    card.appendChild(cardDesc);
    card.appendChild(cardAction);

    card.classList.add("plugin-help-card", "mdl-card", "mdl-shadow--2dp");
    if (isDeprecated(plugin.Description)) {
        card.classList.add("deprecated");
    }
    return card;
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
 * Redraw plugin cards.
 * @param {string} repo repo name.
 * @param {Map<string, Object>} helpMap maps a plugin name to a plugin.
 */
function redrawPlugin(repo: string, helpMap: Map<string, {isExternal: boolean, plugin: PluginHelp}>): void {
    const container = document.querySelector("#plugin-container")!;
    while (container.childElementCount !== 0) {
        container.removeChild(container.firstChild!);
    }
    const names = helpMap.keys();
    const nameArray = Array.from(names).sort();
    nameArray.forEach(name => {
        container.appendChild(createPlugin(repo, name, helpMap.get(name)!))
    });
}

/**
 * Redraws the content of the page.
 */
function redraw(): void {
    const repoSel = selectionText(document.getElementById("repo") as HTMLSelectElement);
    if (window.history && window.history.replaceState !== undefined) {
        if (repoSel !== "") {
            history.replaceState(null, "", "/plugins?repo="
                + encodeURIComponent(repoSel));
        } else {
            history.replaceState(null, "", "/plugins")
        }
    }
    redrawOptions();

    const plugins: Map<string, {isExternal: boolean, plugin: PluginHelp}> = new Map();
    applicablePlugins(repoSel, allHelp.RepoPlugins)
        .forEach((name) => {
            if (allHelp.PluginHelp[name]) {
                plugins.set(
                    name,
                    {
                        isExternal: false,
                        plugin: allHelp.PluginHelp[name]
                    });
            }
        });
    applicablePlugins(repoSel, allHelp.RepoExternalPlugins)
        .forEach((name) => {
            if (allHelp.ExternalPluginHelp[name]) {
                plugins.set(
                    name,
                    {
                        isExternal: true,
                        plugin: allHelp.ExternalPluginHelp[name]
                    });
            }
        });
    redrawPlugin(repoSel, plugins);
}

/**
 * Returns first sentence from plugin's example.
 */
function getFirstSentence(text: string): string {
    const fullStop = text.indexOf(".");
    return fullStop === -1 ? text : text.slice(0, fullStop + 1);
}

/**
 * Returns true if the plugin is deprecated.
 */
function isDeprecated(text: string): boolean {
    const dictionary = ["deprecated!"];
    text = text.toLowerCase();
    for (let i = 0; i < dictionary.length; i++) {
        if (text.indexOf(dictionary[i]) !== -1) {
            return true;
        }
    }
    return false;
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
