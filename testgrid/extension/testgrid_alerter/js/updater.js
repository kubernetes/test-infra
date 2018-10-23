/**
 * @fileoverview Polls and receives alerts from TestGrid server for data on
 * dashboards, tabs, and alerts and stores user config data.
 *
 * TODO(carolinemoore) switch to Tango notifications.
 */

/**
 * Holds information about alerts for a single dashboard.
 */
var Alert = class {
  /**
   * Creates a new Alert object.
   * @param {string} dashboard The name of the dashboard.
   * @param {Array} tabs A list of DashboardTabSummary objects for the tabs in
   *   the given dashboard.
   */
  constructor(dashboard, tabs) {
    this.dashboard = dashboard;
    this.tabs = tabs;
  }
};

/**
 * Holds public functions in updater.js.
 */
var Updater = (function() {

  // Mapping from dashboard name to list of tab names for all dashboards.
  let dashboards = {};
  // Mapping from dashboard name to list of tab names for selected dashboards
  // and tabs.
  let selectedDashboards = {};
  // List of Alerts based on information requested from TestGrid. There will be
  // one alert object for each dashboard.
  let alerts = [];
  // A boolean indicating whether the user is able to request data from
  // TestGrid.
  let hasPermission = false;

  // The base url to use to make requests.
  let URL_BASE = 'https://testgrid.k8s.io/';

  /**
   * Makes a request to the TestGrid server for a list of dashboards and tabs.
   */
  function updateDashboardListFromServer() {
    // URL to make the request to.
    let url = URL_BASE + 'q/list';
    let xhttp = new XMLHttpRequest();
    xhttp.onreadystatechange = function() {
      if (this.readyState == 4) {
        if (this.status == 200) {
          dashboards = JSON.parse(this.responseText);
          hasPermission = true;
          console.log('finished getting dashboards');
        } else {
          hasPermission = false;
        }
      }
    };
    xhttp.open('GET', url, true);
    xhttp.send();
  }

  /**
   * Requests the list of selected dashboards from chrome.storage.
   *
   * Calls updateAlerts() if the selected dashboards have changed.
   */
  function updateSelectedDashboards() {
    try {
      chrome.storage.sync.get('selectedDashboards', function(items) {
        if ('selectedDashboards' in items &&
            selectedDashboards != items.selectedDashboards) {
          selectedDashboards = items.selectedDashboards;
          updateAlerts();
          console.log('selected dashboards have changed');
        }
        console.log('finished getting selected dashboards');
      });
    } catch (err) {
      console.log('caught an error requesting data from chrome.storage');
    }
  }

  /**
   * Creates a DashboardTabSummary object with the given dashboardName and
   * dashboardTabName and with other necessary fields with no real data.
   *
   * For testing only.
   * TODO(carolinemoore) remove.
   *
   * @param {string} dashboardName the name of the dashboard
   * @return {Object} a DashboardTabSummary object
   */
  function createEmptyDashboardTabSummary(dashboardName) {
    return {dashboard_name: dashboardName, alert: '', tests: []};
  }

  /**
   * Requests current alert information from TestGrid.
   *
   * Currently fills in fake data with no failures for testing.
   * TODO(carolinemoore) implement.
   */
  function updateAlerts() {
    console.log('updating alerts');
    alerts = [];
    for (dashboardName in selectedDashboards) {
      if (selectedDashboards[dashboardName].length > 0) {
        tabs = [];
        for (i in selectedDashboards[dashboardName]) {
          tabs[selectedDashboards[dashboardName][i]] =
              (createEmptyDashboardTabSummary(dashboardName));
        }
        alerts.push(new Alert(dashboardName, tabs));
      }
    }
  }

  /**
   * Load data on startup.
   */
  (function() {
    updateDashboardListFromServer();
    updateSelectedDashboards();
    chrome.browserAction.setBadgeBackgroundColor({color: [200, 55, 55, 255]});
  })();

  /**
   * Updates selectedDashboards if selectedDashboards changes elsewhere.
   *
   * Calls updateAlerts() if selectedDashboards chagnes from what is currently
   * stored.
   */
  chrome.storage.onChanged.addListener(function(changes, areaName) {
    console.log(changes);
    if (changes.hasOwnProperty('selectedDashboards') &&
        (changes.selectedDashboards.newValue != selectedDashboards)) {
      selectedDashboards = changes.selectedDashboards.newValue;
      updateAlerts();
    }
  });

  return {
    /**
     * Getter function for alerts.
     * @return {Array} A list of alerts.
     */
    getAlerts: function() {
      return alerts;
    },

    /**
     * Provides values used by the settings page.
     *
     * @return {Array} array containing the list of dashboards, the list of
     *   selected dashboards, and the boolean indicating if the extension
     *   successfully requested data from TestGrid.
     */
    openSettings: function() {
      return [dashboards, selectedDashboards, hasPermission];
    },

    /**
     * Updates and saves selectedDashboards to the given value and calls
     * updateAlerts() if it's value changed.
     *
     * @param {Object} newDashboards Mapping from dashboard name to list of tab
     * names for dashboards and tabs selected by the user.
     */
    saveSettings: function(newDashboards) {
      if (selectedDashboards != newDashboards) {
        selectedDashboards = newDashboards;
        chrome.storage.sync.set(
            {'selectedDashboards': newDashboards}, function() {
              console.log('finished setting selected dashboards');
            });
        updateAlerts();
      }
    }
  };
})();
