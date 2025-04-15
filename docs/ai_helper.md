# AI Helper Document for Audiobook Organizer

## Project Context and Purpose

This document provides essential context about the Audiobook Organizer application to assist AI systems in understanding the project and providing appropriate suggestions and modifications.

### Project Purpose

Audiobook Organizer is designed to solve a common problem for audiobook enthusiasts: organizing audiobook files into coherent series without physically moving or renaming the files. Many users have large audiobook collections with inconsistent naming and organizational structures. This application:

1. Scans directories containing audiobook files
2. Extracts metadata from audio files
3. Uses pattern matching and fuzzy logic to identify series relationships
4. Creates playlists organized by series
5. Updates audio file metadata tags with series information
6. Stores all organizational information in a SQLite database

The primary goal is to help users discover and enjoy series of audiobooks in the correct order without disrupting their existing file organization.

## Design Philosophy and Principles

When working with this codebase, please adhere to these design principles:

1. **Non-destructive operations**: Never modify the original files' locations or names. All organization happens through metadata and database entries.

2. **Fail gracefully**: The application should handle missing or malformed files gracefully, logging warnings rather than failing completely.

3. **Progressive enhancement**: Start with simple pattern matching, then apply more sophisticated techniques only when needed.

4. **Modular architecture**: Each component has a specific responsibility and minimal dependencies on other components.

5. **User control**: Provide options for customization while offering sensible defaults.

## Implementation Guidance

### File Structure and Organization

Maintain the established package structure:
- **cmd/**: Command-line interface components
- **internal/**: Application logic components
- **docs/**: Documentation files

### Code Style Conventions

1. Use descriptive function and variable names that reflect their purpose
2. Document public functions with meaningful comments
3. Handle errors explicitly and provide context in error messages
4. Use consistent formatting (use Go standard formatting)
5. Prefer composition over inheritance
6. Implement dependencies using interfaces for testability

### Database Operations

When working with the SQLite database:
1. Use parameterized queries to prevent SQL injection
2. Keep transactions small and focused
3. Close database resources properly
4. Handle NULL values using sql.NullXxx types
5. Use appropriate indexes for performance

### File Operations

When dealing with audio files:
1. Always open files in read-only mode unless explicitly writing tags
2. Close file handles promptly after use
3. Use filepath.Walk for directory traversal
4. Handle path separators appropriately for cross-platform compatibility
5. Check file existence before operations

### Tag Manipulation

When implementing tag writing:
1. Preserve all existing tags when adding/modifying series tags
2. Implement format-specific tag writing using appropriate libraries/tools
3. Support multiple tag fields for series information (GROUPING, CONTENTGROUP)
4. Back up tag data before modification when possible

## Key Algorithms and Data Structures

### Series Identification

The matcher package contains the core logic for identifying series relationships:

1. Regular expression patterns match common naming conventions
2. Hierarchical matching attempts progressively more flexible techniques
3. Fuzzy string matching compares title similarity
4. Directory structure analysis infers relationships from file organization

When enhancing or modifying this logic:
- Add patterns at the appropriate specificity level
- Test with a diverse range of real-world examples
- Consider false positive vs. false negative tradeoffs
- Document the rationale for complex matching rules

### Database Schema

The database design follows normal form with relationships between:
- Authors (many books per author)
- Series (many books per series, one author per series)
- Books (one series per book)
- Playlists (one series per playlist)
- Playlist items (many books per playlist)

When extending the schema:
- Maintain the foreign key relationships
- Use appropriate data types
- Add indexes for frequently queried columns
- Document schema changes in technical documentation

## Tools and External Dependencies

The application relies on several key libraries:

1. **Cobra/Viper**: For command-line interface and configuration
2. **SQLite3**: For database storage
3. **dhowden/tag**: For reading audio metadata
4. **lithammer/fuzzysearch**: For fuzzy string matching
5. **schollz/progressbar**: For progress visualization

When adding new dependencies:
1. Evaluate license compatibility (prefer MIT/Apache2/BSD)
2. Consider maintenance status and community support
3. Document the purpose of the dependency
4. Properly vendor or manage the dependency with go modules

## Known Limitations and Future Directions

Be aware of these current limitations when suggesting improvements:

1. **Tag Writing**: The current implementation only provides placeholders for tag writing, using external tools. A complete implementation would directly write tags.

2. **External APIs**: Integration with book databases like Goodreads is planned but not yet implemented.

3. **Concurrency**: File processing is currently sequential but could benefit from parallelization.

4. **Web Interface**: A future enhancement could include a web-based UI for visualization and manual organization.

5. **Advanced Matching**: The current matcher could be enhanced with machine learning techniques for better accuracy.

## AI Guidance for Project Tasks

When assisting with this project, please:

1. **Respect existing architecture**: Suggest improvements that fit within the established patterns
2. **Prioritize non-destructive operations**: Never suggest modifications that would rename or move users' original files
3. **Handle edge cases**: Consider uncommon but realistic scenarios in your recommendations
4. **Provide complete solutions**: Include error handling and documentation in suggested code
5. **Explain rationale**: When suggesting changes, explain why they improve the system
6. **Consider resource efficiency**: Audiobook files can be large; be mindful of memory and processing requirements
7. **Support cross-platform operation**: Ensure suggestions work on Windows, macOS, and Linux

## Testing Guidelines

When developing or suggesting tests:

1. Write unit tests for core logic components
2. Create integration tests for end-to-end workflows
3. Use fixtures and mocks for filesystem and database operations
4. Test with a diverse set of real-world filename patterns
5. Include edge cases (empty files, malformed metadata, etc.)
6. Ensure tests clean up after themselves (temporary files, test databases)

## Future Roadmap Context

When considering enhancements, these features are on the roadmap:

1. **Tag Writing Implementation**: Complete the tag writing functionality using direct library calls rather than external tools
2. **Goodreads API Integration**: Add ability to query external book databases for more accurate series information
3. **Multi-Author Series Support**: Enhance the data model to handle series with multiple authors
4. **Audiobook Duration Analysis**: Extract and store playback duration for better playlist information
5. **Cover Art Management**: Extract and organize cover art from audiobook files
6. **User Feedback Loop**: Allow users to correct incorrect series matches to improve future matching
7. **Smart Playlists**: Generate playlists based on genre, narrator, or other criteria

---

This helper document is intended to be a living resource. When suggesting significant changes to the codebase, please consider updating this document to reflect new architectural decisions or design principles.
