#version 450

// Push constant for time (must match Go struct layout)
layout(push_constant) uniform PushConstants {
    float time;
} push;

// Output to fragment shader
layout(location = 0) out vec4 fragColor;

void main() {
    // Triangle vertices
    vec2 positions[3] = vec2[](
        vec2(0.0, -0.5),
        vec2(0.5, 0.5),
        vec2(-0.5, 0.5)
    );

    // Vertex colors (RGB)
    vec4 colors[3] = vec4[](
        vec4(1.0, 0.0, 0.0, 1.0),  // Red
        vec4(0.0, 1.0, 0.0, 1.0),  // Green
        vec4(0.0, 0.0, 1.0, 1.0)   // Blue
    );

    // Rotation matrix
    float angle = push.time;
    float c = cos(angle);
    float s = sin(angle);
    mat2 rot = mat2(c, s, -s, c);

    // Rotate position
    vec2 pos = rot * positions[gl_VertexIndex];
    gl_Position = vec4(pos, 0.0, 1.0);
    fragColor = colors[gl_VertexIndex];
}
