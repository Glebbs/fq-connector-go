FROM ubuntu:22.04 AS builder

SHELL ["/bin/bash", "-c"]
RUN echo 'alias ll="ls -l"' >> /root/.bashrc
RUN echo 'export PS1="\[\033[1;36m\][\u@\h \W]\[\033[0;31m\]$\[\033[0;37m\] "' >> /root/.bashrc

#from https://github.com/greenplum-db/gpdb/blob/6.25.3/README.ubuntu.bash:
RUN --mount=target=/var/lib/apt/lists,type=cache,sharing=locked \
    --mount=target=/var/cache/apt,type=cache,sharing=locked \
    rm -f /etc/apt/apt.conf.d/docker-clean \
    && apt-get update \
    && apt-get -y install \
        bison ccache cmake curl flex git-core gcc g++ inetutils-ping libapr1-dev libbz2-dev \
        libcurl4-gnutls-dev libevent-dev libkrb5-dev libpam-dev libperl-dev libreadline-dev \
        libssl-dev libxml2-dev libyaml-dev libzstd-dev locales net-tools ninja-build \
        openssh-client openssh-server openssl zlib1g-dev libldap-dev libossp-uuid-dev \
        vim wget mc sudo iproute2 \
        python2.7 python2.7-dev \
    && export DEBIAN_FRONTEND=noninteractive && apt-get install -y krb5-kdc krb5-admin-server

RUN curl https://bootstrap.pypa.io/pip/2.7/get-pip.py --output /tmp/get-pip.py && python2.7 /tmp/get-pip.py
RUN pip2.7 install PyYAML==5.3.1 conan==1.48.2 psutil==5.9.6
RUN ln -s /usr/bin/python2.7 /usr/bin/python

# Install pathed version of xerces-c https://github.com/greenplum-db/gp-xerces
WORKDIR /opt/
RUN --mount=target=/opt/cache,type=cache,sharing=locked \
    cd /opt/cache && \
    [ -f v3.1.2-p1.zip ] || wget https://github.com/greenplum-db/gp-xerces/archive/refs/tags/v3.1.2-p1.zip && \
    unzip v3.1.2-p1.zip -d /opt/

WORKDIR /opt/gp-xerces-3.1.2-p1
RUN ./configure --prefix=/usr/local && make -j 4 && make install

WORKDIR /opt/
RUN --mount=target=/opt/cache,type=cache,sharing=locked \
    cd /opt/cache && \
    [ -f 6.25.3.zip ] || wget https://github.com/greenplum-db/gpdb/archive/refs/tags/6.25.3.zip && \
    unzip 6.25.3.zip -d /opt/

WORKDIR /opt/gpdb-6.25.3
RUN echo "6.25.3 build dev" > ./VERSION
RUN ./configure --with-orca --with-libxml --with-uuid=ossp --with-ldap --with-openssl \
      --enable-gpfdist --with-gssapi --with-zstd --prefix=/usr/local/gpdb
RUN make -j 4
RUN make install

WORKDIR /opt/
RUN wget https://github.com/comagic/greenplum_bitmap_bucket/archive/refs/heads/main.zip && \
    unzip main.zip -d /opt/ && mv greenplum_bitmap_bucket-main bitmap_bucket
# -----------------------------------------------------------------------------


FROM ubuntu:22.04 AS slim

SHELL ["/bin/bash", "-c"]
RUN echo 'alias ll="ls -l"' >> /root/.bashrc
RUN echo 'export PS1="\[\033[1;36m\][\u@\h \W]\[\033[0;31m\]$\[\033[0;37m\] "' >> /root/.bashrc

RUN --mount=target=/var/lib/apt/lists,type=cache,sharing=locked \
    --mount=target=/var/cache/apt,type=cache,sharing=locked \
    rm -f /etc/apt/apt.conf.d/docker-clean && \
    apt-get update && \
    apt-get -y install \
      make python2.7 openssh-client openssh-server openssl sudo iproute2 \
      net-tools less inetutils-ping locales libcurl3-gnutls

WORKDIR /opt/
RUN ln -s /usr/bin/python2.7 /usr/bin/python
COPY --from=builder /usr/local/lib/python2.7/dist-packages/psutil /usr/local/lib/python2.7/dist-packages/psutil

RUN sed -i '/en_US.UTF-8/s/^# //g' /etc/locale.gen && locale-gen

RUN --mount=from=builder,source=/usr/local/lib,target=/opt/builder_local_lib/ \
    cd /opt/builder_local_lib/ && \
    cp libxerces-c-3.1.so libxerces-c.a libxerces-c.la libxerces-c.so /usr/local/lib/
RUN echo /usr/local/lib >> /etc/ld.so.conf.d/custom_libs.conf && ldconfig


RUN --mount=from=builder,type=bind,source=/opt/gpdb-6.25.3,target=/opt/gpdb-6.25.3,readwrite \
    cd /opt/gpdb-6.25.3 && make install

RUN --mount=from=builder,source=/opt/bitmap_bucket,target=/opt/bitmap_bucket,readwrite \
     cd /opt/bitmap_bucket && make install

#from https://github.com/DataGrip/docker-env/blob/master/greenplum/6.8/Dockerfile:
RUN mkdir /data \
 && mkdir /data/data1 \
 && mkdir /data/data2 \
 && mkdir /data/master \
 && source /usr/local/gpdb/greenplum_path.sh \
 && cp $GPHOME/docs/cli_help/gpconfigs/gpinitsystem_singlenode /data/ \
 && sed -i 's/gpdata1/data\/data1/g' /data/gpinitsystem_singlenode \
 && sed -i 's/gpdata2/data\/data2/g' /data/gpinitsystem_singlenode \
 && sed -i 's/gpmaster/data\/master/g' /data/gpinitsystem_singlenode

RUN useradd -md /home/gpadmin/ --shell /bin/bash gpadmin \
 && chown gpadmin -R /data \
 && echo "source /usr/local/gpdb/greenplum_path.sh" > /home/gpadmin/.bash_profile \
 && echo 'export PS1="\[\033[1;35m\][\u@\h \W]\[\033[0;31m\]$\[\033[0;37m\] "'  >> /home/gpadmin/.bash_profile \
 && echo 'export MASTER_DATA_DIRECTORY="/data/master/gpsne-1/"'  >> /home/gpadmin/.bash_profile \
 && echo 'alias ll="ls -l"'  >> /home/gpadmin/.bash_profile \
 && chown gpadmin:gpadmin /home/gpadmin/.bash_profile \
 && su - gpadmin bash -c 'mkdir /home/gpadmin/.ssh' \
 && echo "gpadmin ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers \
 && echo "root ALL=NOPASSWD: ALL" >> /etc/sudoers \
 && echo "gpadmin:123456" | sudo chpasswd

RUN mkdir /run/sshd
RUN echo "set hlsearch" >> /home/gpadmin/.vimrc

# from https://github.com/greenplum-db/gpdb/blob/6.21.1/src/tools/docker/ubuntu16_ppa/install_and_start_gpdb.sh:
RUN su - gpadmin bash -c '\
    ssh-keygen -f /home/gpadmin/.ssh/id_rsa -t rsa -N "" && \
    cp /home/gpadmin/.ssh/id_rsa.pub /home/gpadmin/.ssh/authorized_keys && \
    chmod 600 /home/gpadmin/.ssh/authorized_keys'

RUN echo "def gethostname(): return 'localhost'" >> /usr/lib/python2.7/socket.py
RUN mv /usr/bin/hostname{,.bkp} && \
  echo "echo localhost" > /usr/bin/hostname && \
  chmod +x /usr/bin/hostname && \
  echo localhost > /etc/hostname

RUN su - gpadmin bash -c '\
    cd /data/ && \
    cd /data/ && \
    echo "starting sshd ..." && \
    sudo /etc/init.d/ssh start && \
    sleep 2 && \
    ssh -o StrictHostKeyChecking=no localhost ls && \
    ssh -o StrictHostKeyChecking=no `hostname` ls && \
    source /home/gpadmin/.bash_profile && \
    echo localhost > /data/hostlist_singlenode && \
    sed -i "s/hostname_of_machine/localhost/g" /data/gpinitsystem_singlenode && \
    echo "gpssh-exkeys ..." && \
    gpssh-exkeys -f /data/hostlist_singlenode && \
    echo "gpinitsystem ..." && \
    gpinitsystem --ignore-warnings -ac gpinitsystem_singlenode && \
    echo "host all  all 0.0.0.0/0 trust" >> /data/master/gpsne-1/pg_hba.conf && \
    psql -d postgres -c "alter role gpadmin with password \$\$123456\$\$" && \
    echo "gpstop ..." && \
    gpstop -a'

RUN mv /usr/bin/hostname{.bkp,}

COPY entrypoint.sh /
RUN chmod +x /entrypoint.sh
CMD ["/entrypoint.sh"]