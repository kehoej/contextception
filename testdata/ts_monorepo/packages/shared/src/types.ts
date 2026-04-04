/** Core shared types. */

export interface User {
  id: string;
  name: string;
  email: string;
}

export interface Product {
  id: string;
  name: string;
  price: number;
}

export interface ApiResponse<T> {
  data: T;
  status: number;
  error?: string;
}
