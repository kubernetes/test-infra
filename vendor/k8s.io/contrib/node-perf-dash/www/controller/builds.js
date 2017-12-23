
/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

// Parse test information
PerfDashApp.prototype.parseTest = function() {
    angular.forEach(this.tests, function(test) {
        parts = test.split("_");

        treeNode = this.testOptionTreeRoot
        angular.forEach(parts, function(part) {
            if(!(part in treeNode)) {
                treeNode[part] = {}; // new node
            }
            treeNode = treeNode[part]; // next level
        }, this);
    }, this);
    this.testTypes = Object.keys(this.testOptionTreeRoot);
};

// Change test options selection when test type is changed
PerfDashApp.prototype.testTypeChanged = function() {
    if(!this.testType) {
        return;
    }
    this.testSelectedOptions = {}
    this.testOptions = {}

    options = testOptions[this.testType].options;
    treeNode = this.testOptionTreeRoot[this.testType];
    keys = Object.keys(treeNode);
    option = options[0];
    this.testOptions[option] = keys;
    this.testSelectedOptions[option] = keys[0]; // init value   
    this.testOptionChanged(option);
}

// Select test options
PerfDashApp.prototype.testOptionChanged = function(name) {
    // set initial values
    treeNode = this.testOptionTreeRoot[this.testType];
    options = testOptions[this.testType].options;
    toChange = false;
    for(var i in options) {
        option = options[i];
        if(toChange) {
            keys = Object.keys(treeNode);
            this.testOptions[option] = keys;
            if(keys.length == 0) {
                break;
            }
            selected = keys[0]; // init value
            this.testSelectedOptions[option] = selected;
        } else {
            selected = this.testSelectedOptions[option];
        }
        treeNode = treeNode[selected];
        if(option == name) {
            toChange = true;
        }
    }

    this.test = this.testType;
    for(var i in options) {
        option = options[i];
        selected = this.testSelectedOptions[option];
        if(!selected) {
            break;
        }
        //console.log(selected)
        this.test += '_' + selected
    }
    this.testChanged();
    //console.log(this.test)
}

// Parse 'machine' and 'image' labels from 'node'
PerfDashApp.prototype.parseNodeInfo = function() {
    angular.forEach(this.allData, function(test, testName) {
        if(!(testName in this.testNodeTreeRoot)) {
            this.testNodeTreeRoot[testName] = {};
        }

        angular.forEach(test.data, function(nodeData, nodeName) {
            parts = nodeName.split("_");
            if(parts.length > 0) { 
                // formatted node name: "image_machine"
                // TODO(coufon): we need a standard format to name a node using image and machine type.
                image = parts[0];
                machine = parts[1];
            } else {
                console.log("node name format error: " + nodeName)
                return
            }

            // make selection of machine/image/host here
            treeNode = this.testNodeTreeRoot[testName];
            if(!(image in treeNode)) {
                treeNode[image] = {};
            }
            treeNode = treeNode[image];
            if(!(machine in treeNode)) {
                treeNode[machine] = {};
            }
        }, this);
    }, this);

};

// Apply new data to charts, using the selected test, reflect the changes to test options
PerfDashApp.prototype.testChangedWrapper = function() {
    this.testChanged();

    parts = this.test.split('_');

    this.testType = parts[0];
    options = testOptions[this.testType].options;
    treeNode = this.testOptionTreeRoot[this.testType];

    selecteds = parts.slice(1, parts.length);
    for(var i in selecteds) {
        selected = selecteds[i];
        option = options[i];
        this.testSelectedOptions[option] = selected;
        this.testOptions[option] = Object.keys(treeNode);
        treeNode = treeNode[selected]
    }
};

// Apply new data to charts, using the selected test
PerfDashApp.prototype.testChanged = function() {
    if(!this.test | !(this.test in this.allData)) {
        if(this.tests.length > 0) {
            this.test = this.tests[0]
        } else {
            return;
        }
    }
    this.imageList = Object.keys(this.testNodeTreeRoot[this.test]);
    this.imageChanged();
};

PerfDashApp.prototype.imageChanged = function() {
    if(this.image != null && this.imageList.indexOf(this.image) == -1){
        this.image = null;
        this.machine = null;
        this.machineList = [];
    }
    if(this.image == null) {
        if(this.imageList.length > 0) {
            this.image = this.imageList[0];
        } else {
            return;
        }
    }
    this.machineList = Object.keys(this.testNodeTreeRoot[this.test][this.image]);
    this.machineChanged();
}

PerfDashApp.prototype.machineChanged = function() {
    if(this.machine != null && this.machineList.indexOf(this.machine) == -1) {
        this.machine = null;
    }
    if(this.machine ==null) {
        if(this.machineList.length > 0) {
            this.machine = this.machineList[0];
        } else {
            return;
        }
    }
    this.nodeChanged();
}

// Apply new data to charts, using the selected node (machine/image)
PerfDashApp.prototype.nodeChanged = function() {
    if(this.image == null || this.machine == null) {
        return;
    }
    this.node = this.image + '_' + this.machine;

    this.data = this.allData[this.test].data[this.node];
    this.builds = this.getBuilds();
    this.labels = this.getLabels();
    
    newMinBuild = parseInt(Math.min.apply(Math, this.builds));
    newMaxBuild = parseInt(Math.max.apply(Math, this.builds));
    
    this.resetBuildRange();
    this.build = this.maxBuild;
    this.labelChanged();
    this.plotBuildsTracing();
};

// Update the data to charts, using selected labels
PerfDashApp.prototype.labelChanged = function() {
    // get data for each plot
    angular.forEach(plots, function(plot){
        plotRule = plotRules[plot];
        this.seriesDataMap[plot] = [];
        this.seriesMap[plot] = [];
        this.buildsMap[plot] = [];
        switch(plot) {
            case 'latency':
            case 'throughput':
            case 'throughput':
            case 'kubelet_cpu':
            case 'kubelet_memory':
            case 'runtime_cpu':
            case 'runtime_memory':
                selectedLabels = plotRule.labels;
                break;
            default:
                console.log('unkown plot type ' + plot)
                return;              
        }
        //selectedLabels['node'] = this.node;
        result = this.getData(selectedLabels, this.buildsMap[plot]);
        //console.log(JSON.stringify(this.buildsMap[plot]))
        if (Object.keys(result).length <= 0) {
            return;
        }
        // All the unit should be the same
        this.optionsMap[plot] = {
            scales: {
                xAxes: [{
                    scaleLabel: {
                        display: true,
                        labelString: 'Build',
                    }
                }],
                yAxes: [{
                    scaleLabel: {
                        display: true,
                        labelString: result[0].unit,
                    }
                }]
            }, 
            elements: {
                line: {
                    fill: false,
                },
            },
            legend: {
                display: true,
            },
        };
        angular.forEach(plotRule.metrics, function(metric) {
            this.seriesDataMap[plot].push(this.getStream(result, metric));
            this.seriesMap[plot].push(metric);
        }, this);
    }, this)
};

// Get all of the builds for the data set (e.g. build numbers)
PerfDashApp.prototype.getBuilds = function() {
    return Object.keys(this.data)
};

// Get the set of all labels (e.g. 'resources', 'verbs') in the data set
PerfDashApp.prototype.getLabels = function() {
    var set = {};
    angular.forEach(this.data, function(items, build) {
        angular.forEach(items.perf, function(item) {
            angular.forEach(item.labels, function(label, name) {
                if (set[name] == undefined) {
                    set[name] = {}
                }
                set[name][label] = true
            });
        });
    });

    this.selectedLabels = {}
    var labels = {};
    angular.forEach(set, function(items, name) {
        labels[name] = [];
        angular.forEach(items, function(ignore, item) {
            if (this.selectedLabels[name] == undefined) {
              this.selectedLabels[name] = item;
            }
            labels[name].push(item)
        }, this);
    }, this);
    return labels;
};

// Extract a time series of data for specific labels
PerfDashApp.prototype.getData = function(labels, builds) {
    var result = [];
    angular.forEach(this.data, function(items, build) {
        if(parseInt(build) >= this.minBuild && parseInt(build) <= this.maxBuild) {
            angular.forEach(items.perf, function(item) {
                var match = true;
                angular.forEach(labels, function(label, name) {
                    if (item.labels[name] != label) {
                        match = false;
                    }
                });
                if (match && builds[builds.length-1] != build) {
                    result.push(item);
                    builds.push(build)
                }
            });
        }
    }, this);
    return result;
};

// Given a slice of data, turn it into a time series of numbers
// 'data' is an array of APICallLatency objects
// 'stream' is a selector for latency data, (e.g. 'Perc50')
PerfDashApp.prototype.getStream = function(data, stream) {
    var result = [];
    angular.forEach(data, function(value) {
        result.push(value.data[stream]);
    });
    return result;
};

PerfDashApp.prototype.buildRangeChanged = function() {
    this.labelChanged();
};

PerfDashApp.prototype.resetBuildRange = function() {
    this.minBuild = parseInt(Math.min.apply(Math, this.builds));
    this.maxBuild = parseInt(Math.max.apply(Math, this.builds));

    this.buildRangeChanged();
};