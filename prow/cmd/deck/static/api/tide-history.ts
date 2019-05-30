import {Pull} from "../api/prow";

export interface HistoryData {
    History: {[key: string]: Record[]};
}

export interface Record {
  time: string;
  action: string;
  baseSHA?: string;
  target?: Pull[];
  err?: string;
}
