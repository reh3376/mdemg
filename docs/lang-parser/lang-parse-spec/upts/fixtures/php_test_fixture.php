<?php

declare(strict_types=1);

namespace App\Services;

use App\Models\User;
use Illuminate\Support\Collection;

// Top-level constant
const MAX_RETRIES = 3;

// WordPress-style define constant
define('PLUGIN_VERSION', '2.1.0');

/**
 * Cacheable interface for cache-aware services.
 */
interface Cacheable
{
    public function getCacheKey(): string;

    public function getCacheTtl(): int;
}

/**
 * Trait for slug generation.
 */
trait HasSlug
{
    public function generateSlug(string $value): string
    {
        return strtolower(str_replace(' ', '-', $value));
    }

    protected function slugExists(string $slug): bool
    {
        return false;
    }
}

/**
 * Status enum for user accounts.
 */
enum UserStatus: string
{
    case Active = 'active';
    case Inactive = 'inactive';
    case Suspended = 'suspended';
}

/**
 * User service handling user operations.
 */
class UserService implements Cacheable
{
    use HasSlug;

    public const DEFAULT_ROLE = 'member';
    protected const CACHE_PREFIX = 'user_svc';
    private const MAX_BATCH_SIZE = 100;

    // BUG-FIX TEST: multi-line array constant
    private const ALLOWED_TYPES = [
        'admin',
        'editor',
        'viewer',
    ];

    // BUG-FIX TEST: constant with trailing comment
    private const THROTTLE_SECONDS = 900; // 15 minutes

    public string $serviceName = 'UserService';
    protected int $timeout = 30;
    private readonly string $apiKey;

    // BUG-FIX TEST: constructor emitted as method + promoted property extraction
    public function __construct(
        private readonly UserRepository $repository,
    ) {
        $this->apiKey = config('services.user.key');
    }

    public function findById(int $id): ?User
    {
        return $this->repository->find($id);
    }

    public function createUser(array $data): User
    {
        $slug = $this->generateSlug($data['name']);
        return $this->repository->create(array_merge($data, ['slug' => $slug]));
    }

    protected function validateData(array $data): bool
    {
        return isset($data['name']) && isset($data['email']);
    }

    private function hashPassword(string $password): string
    {
        return password_hash($password, PASSWORD_ARGON2ID);
    }

    public static function getVersion(): string
    {
        return '1.0.0';
    }

    // BUG-FIX TEST: reversed static modifier order (Beaver Builder style)
    static public function getInstance(): self
    {
        return new self(new UserRepository());
    }

    // BUG-FIX TEST: reversed static field modifier
    static protected int $instanceCount = 0;

    // BUG-FIX TEST: reference-return method
    public function &getReference(): array
    {
        return $this->data;
    }

    public function getCacheKey(): string
    {
        return self::CACHE_PREFIX . ':' . $this->serviceName;
    }

    public function getCacheTtl(): int
    {
        return 3600;
    }

    // BUG-FIX TEST: heredoc should NOT produce false-positive constants
    public function getScript(): string
    {
        return <<<'JS'
            const el = document.querySelector('.widget');
            const buttons = el.querySelectorAll('button');
        JS;
    }
}

/**
 * Abstract base controller.
 */
abstract class BaseController
{
    abstract public function index(): array;

    public function respond(mixed $data, int $status = 200): array
    {
        return ['data' => $data, 'status' => $status];
    }
}

/**
 * BUG-FIX TEST: bare methods without visibility (WordPress/legacy style)
 */
class LegacyWidget
{
    function render()
    {
        return '<div>widget</div>';
    }

    function update($data)
    {
        return $data;
    }
}

/**
 * BUG-FIX TEST: readonly class (PHP 8.2)
 */
readonly class ValueObject
{
    public function __construct(
        public string $name,
        public int $value,
    ) {}
}

/**
 * BUG-FIX TEST: single-line constructor with promoted properties
 */
class Notification
{
    public function __construct(public string $message, public int $priority = 0) {}
}

/**
 * Standalone helper function.
 */
function formatCurrency(float $amount, string $currency = 'USD'): string
{
    return number_format($amount, 2) . ' ' . $currency;
}

/**
 * BUG-FIX TEST: indented guarded function
 */
if (! function_exists('calculateDiscount')) {
    function calculateDiscount(float $price, float $percent): float
    {
        return $price * (1 - $percent / 100);
    }
}

// WordPress-style hooks
add_action('init', 'custom_post_type_init');
add_filter('the_content', 'filter_content_output');
