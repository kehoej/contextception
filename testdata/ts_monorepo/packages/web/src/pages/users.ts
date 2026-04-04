/** Users page. */

import { logger } from "@myapp/shared";
import { UserList } from "../components/UserList";

export function UserPage(users: any[]) {
  logger.info("Rendering users page");
  return `<div>${UserList(users)}</div>`;
}
