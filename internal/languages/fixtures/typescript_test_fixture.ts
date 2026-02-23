// Line 1: TypeScript Parser Test Fixture
// Tests all symbol extraction capabilities

// Line 4: Constant (UPPER_CASE)
export const MAX_RETRIES = 3;

// Line 7: Type alias
export type UserId = string;

// Line 10: Interface with extension
export interface UserDto {
  id: UserId;
  name: string;
  email?: string;
}

// Line 17: Enum
export enum UserStatus {
  ACTIVE = 'active',
  INACTIVE = 'inactive',
  PENDING = 'pending'
}

// Line 24: Decorated class
@Injectable()
export class UserService {
  // Line 27: Private property
  private readonly logger: Logger;

  // Line 30: Constructor
  constructor(private userRepo: UserRepository) {
    this.logger = new Logger(UserService.name);
  }

  // Line 35: Async method with decorator
  @Transactional()
  async findById(id: UserId): Promise<UserDto | null> {
    return this.userRepo.findOne(id);
  }

  // Line 41: Method with multiple params
  async create(data: CreateUserInput): Promise<UserDto> {
    return this.userRepo.create(data);
  }

  // Line 46: Static method
  static validateEmail(email: string): boolean {
    return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);
  }
}

// Line 52: Top-level function
export function formatUser(user: UserDto): string {
  return `${user.name} <${user.email}>`;
}

// Line 57: Arrow function
export const validateId = (id: string): boolean => {
  return id.length > 0;
};

// Line 62: Abstract class
export abstract class BaseEntity {
  abstract getId(): string;
}

// Line 67: Class extending another
export class AdminUser extends BaseEntity implements UserDto {
  id!: string;
  name!: string;
  
  getId(): string {
    return this.id;
  }
}
