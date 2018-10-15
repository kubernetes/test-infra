/**
 * @fileoverview Defines behavior of the settings page.
 */

(function() {

// Mapping from dashboard name to list of checkboxes (one for each tab).
let tabCheckLists = {};
// List of checkboxes (one for each dashboard).
let dashboardCheckboxes = [];
// The object to use to get information from updater.
let updater = chrome.extension.getBackgroundPage().Updater;

document.addEventListener('DOMContentLoaded', function() {
  /* extension navigation buttons */
  // Element representing the button to get back to the alerts page without
  // saving.
  let back = document.getElementById('back');
  back.addEventListener('click', backOnClick);
  // Element representing the button to save settings and return to the alerts
  // page.
  let save = document.getElementById('save');
  save.addEventListener('click', saveOnClick);

  // List of information requested from updater.js:
  // [dashboards, selectedDashboards, hasPermission]
  let info = updater.openSettings();

  /* add dashboards */
  // Element holding the Select Dashboards section header
  let selectDashboards = document.getElementById('select-dashboards');
  // Element representing the div that will contain the list of dashboards and
  // tabs.
  let dashboardsDiv = document.getElementById('dashboards-div');
  selectDashboards.addEventListener('click', function() {
    dashboardsDiv.classList.toggle('collapsed');
  });

  // Mapping from dashboard name to list of tab names for all dashboards.
  let dashboards = info[0];
  // Mapping from dashboard name to list of tab names for selected dashboards
  // and tabs.
  let selectedDashboards = info[1];
  // A boolean indicating whether the user is able to request data from
  // TestGrid. If not, the extension displays an error message instead of data.
  let hasPermission = info[2];
  if (hasPermission) {
    // Element representing the Expand All button in the Select Dashboards
    // section. Used to expand all dashboards to show all tabs.
    let expandButton = createButton('Expand All', dashboardsDiv);
    // Element representing the Collapse All button in the Select Dashboards
    // section. Used to collapse all dashboards to hide all tabs.
    let collapseButton = createButton('Collapse All', dashboardsDiv);
    // Array containing a list of divs for each dashboard and a list of
    // dictionaries for each dashboard containing the checkboxes for their tabs.
    let lists = addDashboards(dashboards, selectedDashboards, dashboardsDiv);
    expandButton.addEventListener('click', expandAll.bind(null, lists[0]));
    collapseButton.addEventListener('click', collapseAll.bind(null, lists[0]));
    dashboardCheckboxes = lists[1];
  } else {
    // Element to display error message if the user cannot access TestGrid.
    let error = document.createElement('p');
    error.textContent = 'You do not have permission to access TestGrid';
    dashboardsDiv.appendChild(error);
    console.log(dashboards.length);
  }

});

/**
 * Creates a button with the given text and adds it to the given div.
 * @param {string} text displayed on the button
 * @param {Element} div to add the button to
 * @return {Element} the button
 */
function createButton(text, div) {
  let button = document.createElement('button');
  button.type = 'button';
  button.textContent = text;
  div.appendChild(button);
  return button;
}

/**
 * Switches the current page to popup.html.
 */
function backOnClick() {
  document.location = 'popup.html';
}

/**
 * Send the currently selected settings information to updater.py and switches
 * the current page to popup.html.
 */
function saveOnClick() {
  // Mapping from dashboard name to list of tab names for selected dashboards
  // and tabs.
  let selectedDashboards = {};
  dashboardCheckboxes.forEach(function(tabCheckboxes) {
    // List of the names of selected tabs in this dashboard.
    let tabs = [];
    tabCheckboxes.tabs.forEach(function(checkbox) {
      if (checkbox.checked) {
        tabs.push(checkbox.id);
      }
    });
    if (tabs.length > 0) {
      selectedDashboards[tabCheckboxes.dashboard] = tabs;
    }
  });
  updater.saveSettings(selectedDashboards);
  document.location = 'popup.html';
}

/**
 * Adds dashboards and their tabs to the Select Dashboards section. Selected
 * tabs and dashboards with selected tabs in them start checked.
 * @param {Array} dashboards List of Dashboard objects.
 * @param {Object} selectedDashboards dictionary from dashboard name to list of
 *   tab names for selected dashboards and tabs.
 * @param {Element} dashboardsDiv The div to add the dashboards and tabs to.
 * @return {Array} list of divs for each dashboard, list of dictionaries for
 *   each dashboard containing the checkboxes for their tabs
 */
function addDashboards(dashboards, selectedDashboards, dashboardsDiv) {
  // List of divs containing tabs, one for each dashboard
  let divList = [];
  let checkboxes = [];
  for (dashboardName in dashboards) {
    // Array containing the checkbox and text elements for the dashboard
    let dashboard = addLine(dashboardName, dashboardsDiv);
    dashboard[0].addEventListener(
        'click', toggleDashboard.bind(null, dashboardName));
    // Object containing the dashboard name and a list of tab checkboxes for
    // this dashboard.
    let boxes = {'dashboard': dashboardName, 'tabs': []};
    // Element representing the div that tabs in this dashboard are listed in.
    let tabs = document.createElement('div');
    tabs.id = dashboardName + '-div';
    tabs.classList.add('dashboard-div');
    tabs.classList.add('collapsed');
    dashboardsDiv.appendChild(tabs);
    divList.push(tabs);
    dashboard[1].addEventListener('click', function() {
      tabs.classList.toggle('collapsed');
    });
    dashboards[dashboardName].forEach(function(name) {
      // Array containing the checkbox and text elements for the tab
      let tab = addLine(name, tabs);
      boxes.tabs.push(tab[0]);
      if (dashboardName in selectedDashboards &&
          selectedDashboards[dashboardName].includes(name)) {
        tab[0].checked = true;
      }
    });
    if (dashboardName in selectedDashboards) {
      dashboard[0].checked = true;
    }
    checkboxes.push(boxes);
    tabCheckLists[dashboardName] = boxes.tabs;
  }
  return [divList, checkboxes];
}

/**
 * Adds a line with a checkbox and a label for the given name.
 *
 * The id of the checkbox is set to name.
 *
 * @param {string} name The name of the tab or dashboard being indicated.
 * @param {Element} div The div to add the line to.
 * @return {Array} An array containing the checkbox and the paragraph objects.
 */
function addLine(name, div) {
  // Div to hold the checkbox and text element.
  let line = document.createElement('div');
  div.appendChild(line);
  let box = document.createElement('input');
  box.id = name;
  box.type = 'checkbox';
  box.classList.add('line');
  line.appendChild(box);
  let text = document.createElement('p');
  text.textContent = name;
  text.classList.add('line');
  line.appendChild(text);
  return [box, text];
}

/**
 * Expands every div contained in divList.
 *
 * @param {Array} divList A list of div elements.
 */
function expandAll(divList) {
  divList.forEach(function(div) {
    div.classList.remove('collapsed');
  });
}

/**
 * Collapses every div contained in divList.
 *
 * @param {Array} divList A list of div elements.
 */
function collapseAll(divList) {
  divList.forEach(function(div) {
    div.classList.add('collapsed');
  });
}

/**
 * Sets every tab checkbox for the dashboard with the given dashboard name to
 * the same value as the dashboard's checkbox.
 *
 * @param {string} dashboardName The name of the dashboard whose tab's
 *   checkboxes should be toggled
 */
function toggleDashboard(dashboardName) {
  let set = document.getElementById(dashboardName).checked;
  tabCheckLists[dashboardName].forEach(function(tab) {
    tab.checked = set;
  });
}

})();
