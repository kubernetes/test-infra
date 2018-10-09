// Add the deprecated IE-specific clipboardData to Window.
interface Window {
  clipboardData?: {
    setData: (format: "Text" | "URL", data: string) => boolean,
    getData: (format: "Text" | "URL") => string,
    clearData: (format: "Text" | "URL" | "File" | "HTML" | "Image") => boolean,
  };
}

// Enough typing for the Material Design library to be usable.
interface MaterialSnackbarOptionsNoAction {
  message: string;
  timeout?: number;
}

interface MaterialSnackbarOptionsWithAction {
  actionHandler: (event: Event) => null;
  actionText: string;
}

type MaterialSnackbarOptions = MaterialSnackbarOptionsNoAction | MaterialSnackbarOptionsWithAction;

interface MaterialSnackbar {
  showSnackbar(options: MaterialSnackbarOptions): void;
}

type SnackbarElement<T extends HTMLElement = HTMLElement> = T & {MaterialSnackbar: MaterialSnackbar};
