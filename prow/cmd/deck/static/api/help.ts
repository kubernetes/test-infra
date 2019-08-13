export interface Command {
  Usage: string;
  Featured: boolean;
  Description: string;
  Examples: string[];
  WhoCanUse: string;
}

export interface PluginHelp {
  Description: string;
  Config: {[key: string]: string};
  Events: string[];
  Commands: Command[];
}

export interface Help {
  AllRepos: string[];
  RepoPlugins: {[key: string]: string[]};
  RepoExternalPlugins: {[key: string]: string[]};
  PluginHelp: {[key: string]: PluginHelp};
  ExternalPluginHelp: {[key: string]: PluginHelp};
}
