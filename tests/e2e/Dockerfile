FROM python:3.12-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    curl \
    sudo

# Create non-root user with sudo access
RUN useradd -m -s /bin/bash tester && \
    echo "tester ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers
USER tester
WORKDIR /home/tester

# Copy dots source and install
COPY --chown=tester:tester . /home/tester/dots-src
RUN pip install --user /home/tester/dots-src && \
    echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc

ENV PATH="/home/tester/.local/bin:${PATH}"

# Verify install
RUN dots --version

COPY --chown=tester:tester tests/e2e/scenarios /home/tester/scenarios

ENTRYPOINT ["/bin/bash"]
