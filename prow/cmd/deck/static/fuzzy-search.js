/*
Copyright 2018 The Kubernetes Authors.

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

(function(window) {
  var FuzzySearch = (function() {
    var instance;

    function init(dictionary) {
      dictionary.sort();
      /** {Array<string>} **/
      var dict = dictionary;
      /**
       * Returns true if the string contains all the pattern characters.
       * @param {string} pttn
       * @param {string} str
       * @return {boolean}
       */
      function basicMatch(pttn, str) {
        var i = 0, j = 0;
        while (i < pttn.length && j < str.length) {
          if (pttn[i].toLowerCase() === str[j].toLowerCase()) i += 1;
          j += 1;
        }
        return i === pttn.length;
      }

      /**
       * Sorts dict function. The higher the score, the lower index the string is. If two
       * strings have the same score, sort by alphabetical order.
       * @param {string} a
       * @param {string} b
       * @return {number}
       */
      function sortFn(a, b) {
        if (a.score === b.score) {
          return a.str < b.str ? -1 : (a.str > b.str ? 1 : 0);
        }
        return a.score > b.score ? -1 : 1;
      }

      /**
       * Calculates the score that a matching can get. The string is calculated based on 4
       * criteria:
       *  1. +3 score for the matching that occurs near the beginning of the string.
       *  2. +5 score for the matching that is not an alphabetical character.
       *  3. +3 score for the matching that the string character is upper case.
       *  4. +10 score for the matching that matches the uppercase which is just before a
       *  separator.
       * @param {number} i
       * @param {string} str
       * @return {number}
       */
      function calcScore(i, str) {
        var score = 0;
        var isNotAlphabetical = function (c) {
          return (c < 65 || (c > 90 && c < 97) || c > 122);
        };
        // Bonus if the matching is near the start of the string
        if (i < 3) {
          score += 3;
        }
        // Bonus if the matching is not a alphabetical character
        if (isNotAlphabetical(str.charCodeAt(i))) {
          score += 5;
        }
        // Bonus if the matching is an UpperCase character
        if (str[i].toUpperCase() === str[i]) {
          score += 3;
        }

        // Bonus if matching after a separator
        var separatorBehind = (i === 0 || isNotAlphabetical(str.charCodeAt(i - 1)));
        if (separatorBehind) {
          score += 10;
          score += (str[i].toUpperCase() === str[i] ? 5 : 0);
        }
        return score;
      }

      /**
       * Get maximum score that a string can get against the pattern.
       * @param {string} pttn
       * @param {string} str
       * @return {number}
       */
      function getMaxScore(pttn, str) {
        var score = [];
        for (var i = 0; i < 2; i++) {
          score[i] = [];
          for (var j = 0 ; j < str.length; j++) {
            score[i][j] = 0;
          }
        }
        for (i = 0; i < pttn.length; i++) {
          var t = i % 2;
          for (j = 0; j < str.length; j++) {
            var scoreVal = pttn[i].toLowerCase() === str[j].toLowerCase() ? calcScore(j, str) : 0;
            if (i === 0) {
              score[t][j] = scoreVal;
              if (j > 0) score[t][j] = Math.max(score[t][j], score[t][j-1]);
            } else {
              score[t][j] = score[Math.abs(t-1)][j];
              if (j > 0) {
                score[t][j] = Math.max(score[t][j], score[t][j-1]);
                score[t][j] = Math.max(score[t][j], score[Math.abs(t-1)][j-1] + scoreVal);
              }
            }
          }
        }
        return score[(pttn.length - 1) % 2][str.length - 1];
      }

      // Public methods
      return {
        /**
         * Returns a list of string from dictionary that matches against the pattern.
         * @param {string} pattern
         * @return {Array<string>}
         */
        search: function(pattern) {
          if (!dict || !dict.length === 0) {
                return [];
          }
          if (!pattern || pattern.length === 0) {
            return dict;
          }
          var dictScr = [];
          for (var i = 0; i < dict.length; i++) {
            if (basicMatch(pattern, dict[i])) {
              dictScr.push({
                str: dict[i],
                score: getMaxScore(pattern, dict[i])
              });
            }
          }
          dictScr.sort(sortFn);

          var result = [];
          for (i = 0; i < dictScr.length; i++) {
            if (dictScr[i].score === 0) continue;
            result.push(dictScr[i].str);
          }
          return result;
        },
        /**
         * Sets the dictionary for the fuzzy search.
         * @param {Array<string>} dict
         */
        setDict: function(dictionary) {
          dictionary.sort();
          dict = dictionary;
        }
      };
    }
    return {
      getInstance: function(dictionary = []) {
        if (!instance) {
          instance = init(dictionary);
        }
        return instance;
      }
    };
  })();

  window.FuzzySearch = FuzzySearch;
})(window);
