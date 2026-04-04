/** Request logging middleware. */

import { APP_NAME } from "../config";

export function requestLogger(handler: Function) {
  return (req: any, res: any) => {
    const start = Date.now();
    const result = handler(req, res);
    const duration = Date.now() - start;
    console.log(`[${APP_NAME}] ${req.method} ${req.url} - ${duration}ms`);
    return result;
  };
}
