/** User list component. */

import type { User } from "@myapp/shared";

export function UserList(users: User[]) {
  return users.map((u) => `<li>${u.name} (${u.email})</li>`).join("");
}
