/**
 * @fileoverview Defines behavior of the opening popup page.
 */
(function() {

// List of divs that hold information about tabs. There will be one for each
// dashboard.
let dashboardDivList = [];
// List of divs that hold information about tests. There will be one for each
// tab
let tabDivList = [];
// The object to use to get information from updater.
let updater = chrome.extension.getBackgroundPage().Updater;

document.addEventListener('DOMContentLoaded', function() {
  // Element to contain any warning text, such as if there are no selected
  // dashboards
  let warning = document.getElementById('warning');

  // Element representing the Expand All button. Used to expand all dashboards
  // and tabs to make their alerts visble.
  let expandAllButton = document.getElementById('expand');
  expandAllButton.addEventListener('click', expandAll);
  // Element representing the Collapse All button. Used to collapse all
  // dashboards and tabs to hide their alerts.
  let collapseAllButton = document.getElementById('collapse');
  collapseAllButton.addEventListener('click', collapseAll);
  // Element representing the Expand All Dashboards button. Used to expand all
  // dashboards but leaves tabs collapsed such that tabs are visible but tests
  // are not.
  let expandDashboardsButton = document.getElementById('expand-dashboards');
  expandDashboardsButton.addEventListener('click', expandDashboards);
  // Element representing the Collapse All Dashboards button. Used to collapse
  // all dashboards but leaves tabs in their current state such that when the
  // dashboard is re-expanded the tabs will maintain their state.
  let collapseDashboardsButton = document.getElementById('collapse-dashboards');
  collapseDashboardsButton.addEventListener('click', collapseDashboards);

  // List of Alerts gathered by updater.js. There will be one alert object for
  // each dashboard.
  let alerts = updater.getAlerts();
  if (alerts.length == 0) {
    warning.textContent =
        'You don\'t have any dashboards or tabs selected. Open Select Dashboards under Settings to choose dashboards to follow';
  }
  // Element representing the div to put alert information in.
  let alertsDiv = document.getElementById('alerts');
  for (let i = 0; i < alerts.length; i++) {
    addDashboard(alerts[i], alertsDiv);
  }

  // Element representing the button that takes the user to the settings page.
  let settings = document.getElementById('settings');
  settings.addEventListener('click', settingsOnClick);
});

/**
 * Switches the current page to settings.html.
 */
function settingsOnClick() {
  document.location = 'settings.html';
}

/**
 * Adds information about a selected dashboard, its tabs, and their tests.
 * @param {Alert} alert: An Alert object for a dashboard.
 * @param {Element} alertsDiv The div to put the information in.
 */
function addDashboard(alert, alertsDiv) {
  console.log(alert);
  // Element to hold the dashboard name.
  let dashboardHeader = addHeader(alert.dashboard, 'dashboard', alertsDiv);
  // Element representing the div to put all information (tabs and tests)
  // about this dashboard in.
  let dashboardDiv = addHolder(
      alertsDiv, alert.dashboard, 'dashboard-div', dashboardHeader,
      dashboardDivList);

  for (tabName in alert.tabs) {
    // The DashboardTabSummary for the given tab
    let tabSummary = alert.tabs[tabName];
    // The number of failing tests in the current tab.
    let numTests = tabSummary.tests.length;

    let tabText =
        tabName + ' has ' + numTests + ' failing tests: ' + tabSummary.alert;
    // Element to hold the tab name and tab-level alerts.
    let tabHeader = addHeader(tabText, 'tab', dashboardDiv);
    if (numTests > 0) {
      // Element representing the div to put all test information about this
      // tab in.
      let tabDiv = addHolder(
          dashboardDiv, alert.dashboard + '-' + tabName, 'tab-div', tabHeader,
          tabDivList);
      addTestList(tabDiv, tabSummary.tests);
    }
    addAlertColors(tabSummary.alert, numTests, tabHeader, dashboardHeader);
  }
}

function addTestList(tabDiv, testList) {
  // The unordered list element to hold all test information for this tab.
  let list = document.createElement('ul');
  tabDiv.appendChild(list);
  list.classList.add('tab_ul');

  testList.forEach(function(testSummary) {
    // The list item element that displays information about this test.
    let test = document.createElement('li');
    test.textContent = 'Test ' + testSummary.display_name + ' has failed ' +
        testSummary.fail_count + ' times.';
    list.appendChild(test);
    test.classList.add('test-li');
  });
}

function addHeader(text, className, div) {
  let header = document.createElement('p');
  header.classList.add(className);
  header.textContent = text;
  div.appendChild(header);
  return header;
}

function addHolder(container, id, className, header, list) {
  let div = document.createElement('div');
  container.appendChild(div);
  div.id = id + '-div';
  div.classList.add(className);
  div.classList.add('collapsed');
  header.addEventListener('click', function() {
    div.classList.toggle('collapsed');
  });
  list.push(div);
  return div;
}

function addAlertColors(alert, numTests, tabHeader, dashboardHeader) {
  if (alert == 'Warning: Test results are stale.') {
    tabHeader.classList.add('stale');
  } else if (numTests > 0) {
    tabHeader.classList.add('failing');
  }
  if (alert == 'Warning: Test results are stale.' || numTests > 0) {
    dashboardHeader.classList.add('failing');
  }
}

/**
 * Expand all dashboards and their tabs to make alerts visible.
 */
function expandAll() {
  expandDashboards();
  tabDivList.forEach(function(div) {
    div.classList.remove('collapsed');
  });
}

/**
 * Collapse all dashboards and tabs to hide tabs and alerts.
 */
function collapseAll() {
  collapseDashboards();
  tabDivList.forEach(function(div) {
    div.classList.add('collapsed');
  });
}

/**
 * Expand all dashboards to make tabs visible but not alerts.
 */
function expandDashboards() {
  dashboardDivList.forEach(function(div) {
    div.classList.remove('collapsed');
  });
}

/**
 * Collapse all dashboards to hide information but leave tabs in their current
 * state when the dashboard is re-opened.
 */
function collapseDashboards() {
  dashboardDivList.forEach(function(div) {
    div.classList.add('collapsed');
  });
}
})();
