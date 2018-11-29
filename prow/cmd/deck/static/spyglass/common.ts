export interface BaseMessage {
  type: string;
}

function isBaseMessage(data: any): data is BaseMessage {
  return typeof data.type === 'string';
}

export interface ContentUpdatedMessage extends BaseMessage{
  type: 'contentUpdated';
  height: number;
}

export interface RequestMessage extends BaseMessage {
  type: 'request';
  data: string;
}

export interface RequestPageMessage extends BaseMessage {
  type: 'requestPage';
  data: string;
}

export interface UpdatePageMessage extends BaseMessage {
  type: 'updatePage';
  data: string;
}

export interface Response extends BaseMessage {
  type: 'response';
  data: string;
}

export function isResponse(data: any): data is Response {
  return isBaseMessage(data) && data.type === 'response';
}

export type Message = ContentUpdatedMessage | RequestMessage | RequestPageMessage | UpdatePageMessage | Response;

export interface TransitMessage {
  id: number;
  message: Message;
}

export function isTransitMessage(data: any): data is TransitMessage {
  return typeof data.id === 'number' && data.message && typeof data.message.type === 'string';
}