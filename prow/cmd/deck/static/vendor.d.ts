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
