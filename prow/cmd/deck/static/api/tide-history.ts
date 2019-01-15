
export interface HistoryData {
    History: {[key: string]: Record[]};
}

export interface Record {
	time:    string;
	action:  string;
	baseSHA?: string;
	target?:  PRMeta[];
	err?:     string;
}

export interface PRMeta {
	num:    number;
	author: string;
	title:  string;
	SHA:    string;
}
